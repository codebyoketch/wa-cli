package cmd

// NOTE: written without a working Go 1.25 toolchain in the authoring
// sandbox (see internal/whatsapp/client_test.go's header comment for
// why) — syntax-checked with gofmt only. Please run
// `go test ./cmd/... -run 'TestIsAllDigits|TestResolve|TestRecordSentMessage' -v` locally.

import (
	"io"
	"log/slog"
	"testing"

	"github.com/codebyoketch/wa-cli/internal/app"
	"github.com/codebyoketch/wa-cli/internal/chatstore"
	"github.com/codebyoketch/wa-cli/internal/config"
	"github.com/codebyoketch/wa-cli/internal/msgstore"
)

func TestFindConfigField_CaseInsensitive(t *testing.T) {
	if _, ok := findConfigField("LOGLEVEL"); !ok {
		t.Error("expected case-insensitive match for LOGLEVEL")
	}
	if _, ok := findConfigField("logLevel"); !ok {
		t.Error("expected exact-case match for logLevel")
	}
	if _, ok := findConfigField("notARealKey"); ok {
		t.Error("expected no match for an unknown key")
	}
}

func TestConfigFields_GetSetRoundTrip(t *testing.T) {
	cases := []struct {
		field string
		value string
	}{
		{"logLevel", "debug"},
		{"jsonOutput", "true"},
		{"dataDir", "/tmp/somewhere"},
		{"maxMessagesPerMinute", "42"},
		{"maxMessagesPerHour", "100"},
		{"maxMessagesPerDay", "500"},
		{"confirmNewRecipients", "false"},
		{"notifyEnabled", "true"},
		{"notifyGroups", "false"},
		{"notifyShowPreview", "true"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			f, ok := findConfigField(tc.field)
			if !ok {
				t.Fatalf("field %q not found in configFields", tc.field)
			}
			var c config.Config
			if err := f.set(&c, tc.value); err != nil {
				t.Fatalf("set(%q): unexpected error: %v", tc.value, err)
			}
			if got := f.get(c); got != tc.value {
				t.Errorf("get() after set(%q) = %q, want %q", tc.value, got, tc.value)
			}
		})
	}
}

func TestConfigFields_LogLevel_RejectsInvalidValue(t *testing.T) {
	f, ok := findConfigField("logLevel")
	if !ok {
		t.Fatal("logLevel field not found")
	}
	var c config.Config
	if err := f.set(&c, "verbose"); err == nil {
		t.Error("expected error for invalid logLevel value, got nil")
	}
}

func TestConfigFields_DataDir_RejectsEmpty(t *testing.T) {
	f, ok := findConfigField("dataDir")
	if !ok {
		t.Fatal("dataDir field not found")
	}
	var c config.Config
	if err := f.set(&c, ""); err == nil {
		t.Error("expected error for empty dataDir, got nil")
	}
}

func TestBoolSetter_RejectsInvalidValue(t *testing.T) {
	f, ok := findConfigField("jsonOutput")
	if !ok {
		t.Fatal("jsonOutput field not found")
	}
	var c config.Config
	if err := f.set(&c, "yes"); err == nil {
		t.Error("expected error for non-bool value \"yes\", got nil")
	}
}

func TestIntSetter_RejectsInvalidValue(t *testing.T) {
	f, ok := findConfigField("maxMessagesPerHour")
	if !ok {
		t.Fatal("maxMessagesPerHour field not found")
	}
	var c config.Config
	if err := f.set(&c, "a lot"); err == nil {
		t.Error("expected error for non-numeric value, got nil")
	}
}

func TestCompleteConfigKeys_PrefixMatch(t *testing.T) {
	got := completeConfigKeys("notify")
	want := map[string]bool{"notifyEnabled": true, "notifyGroups": true, "notifyShowPreview": true}
	if len(got) != len(want) {
		t.Fatalf("completeConfigKeys(\"notify\") = %v, want 3 matches from %v", got, want)
	}
	for _, name := range got {
		if !want[name] {
			t.Errorf("unexpected match %q for prefix \"notify\"", name)
		}
	}
}

func TestCompleteConfigKeys_CaseInsensitivePrefix(t *testing.T) {
	got := completeConfigKeys("LOG")
	found := false
	for _, name := range got {
		if name == "logLevel" {
			found = true
		}
	}
	if !found {
		t.Errorf("completeConfigKeys(\"LOG\") = %v, want it to include \"logLevel\"", got)
	}
}

func TestCompleteConfigKeys_NoMatch(t *testing.T) {
	got := completeConfigKeys("zzz_nonexistent")
	if len(got) != 0 {
		t.Errorf("expected no matches, got %v", got)
	}
}
// withAppDataDir points the package-level `a` at a fresh temp data dir
// for the duration of the test, so resolveJID/resolveChat/etc. hit an
// isolated chatstore instead of whatever's on the machine running the
// tests. Log is set to a discard logger so any warning-path logging
// doesn't panic on a nil *slog.Logger.
func withAppDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := a
	a = &app.App{
		Config: config.Config{DataDir: dir},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	t.Cleanup(func() { a = orig })
	return dir
}

// ---- isAllDigits ----

func TestIsAllDigits(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"254700282181", true},
		{"", true}, // vacuously true: no non-digit characters found
		{"12a34", false},
		{"+254700282181", false},
		{"12 34", false},
	}
	for _, tc := range cases {
		if got := isAllDigits(tc.in); got != tc.want {
			t.Errorf("isAllDigits(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// ---- resolveJID ----

func TestResolveJID_LiteralJID(t *testing.T) {
	withAppDataDir(t)
	jid, err := resolveJID("254700282181@s.whatsapp.net")
	if err != nil {
		t.Fatalf("resolveJID: unexpected error: %v", err)
	}
	if jid.String() != "254700282181@s.whatsapp.net" {
		t.Errorf("resolveJID = %q, want %q", jid.String(), "254700282181@s.whatsapp.net")
	}
}

func TestResolveJID_KnownChatName(t *testing.T) {
	dir := withAppDataDir(t)
	cs := chatstore.New(dir)
	if err := cs.Upsert(chatstore.Chat{JID: "254700282181@s.whatsapp.net", Name: "Alice"}); err != nil {
		t.Fatalf("seeding chatstore: %v", err)
	}

	jid, err := resolveJID("Alice")
	if err != nil {
		t.Fatalf("resolveJID: unexpected error: %v", err)
	}
	if jid.String() != "254700282181@s.whatsapp.net" {
		t.Errorf("resolveJID(\"Alice\") = %q, want %q", jid.String(), "254700282181@s.whatsapp.net")
	}
}

func TestResolveJID_BarePhoneNumber(t *testing.T) {
	withAppDataDir(t)
	jid, err := resolveJID("254700282181")
	if err != nil {
		t.Fatalf("resolveJID: unexpected error: %v", err)
	}
	if jid.String() != "254700282181@s.whatsapp.net" {
		t.Errorf("resolveJID = %q, want %q", jid.String(), "254700282181@s.whatsapp.net")
	}
}

func TestResolveJID_PhoneNumberWithPlusPrefix(t *testing.T) {
	withAppDataDir(t)
	jid, err := resolveJID("+254700282181")
	if err != nil {
		t.Fatalf("resolveJID: unexpected error: %v", err)
	}
	if jid.String() != "254700282181@s.whatsapp.net" {
		t.Errorf("resolveJID = %q, want %q", jid.String(), "254700282181@s.whatsapp.net")
	}
}

func TestResolveJID_NoMatch(t *testing.T) {
	withAppDataDir(t)
	_, err := resolveJID("not a phone number or known chat")
	if err == nil {
		t.Fatal("expected error for unresolvable target, got nil")
	}
}

// ---- resolveMessageRef ----

func TestResolveMessageRef_ByNumberedIndex(t *testing.T) {
	dir := withAppDataDir(t)
	ms := msgstore.New(dir)
	jid := "254700282181@s.whatsapp.net"
	_ = ms.Append(msgstore.Message{ID: "m1", ChatJID: jid, Text: "first", Timestamp: 100})
	_ = ms.Append(msgstore.Message{ID: "m2", ChatJID: jid, Text: "second", Timestamp: 200})

	got, err := resolveMessageRef(ms, jid, "2")
	if err != nil {
		t.Fatalf("resolveMessageRef: unexpected error: %v", err)
	}
	if got.Text != "second" {
		t.Errorf("resolveMessageRef(\"2\").Text = %q, want %q", got.Text, "second")
	}
}

func TestResolveMessageRef_ByMessageID(t *testing.T) {
	dir := withAppDataDir(t)
	ms := msgstore.New(dir)
	jid := "254700282181@s.whatsapp.net"
	_ = ms.Append(msgstore.Message{ID: "abc123", ChatJID: jid, Text: "hello", Timestamp: 100})

	got, err := resolveMessageRef(ms, jid, "abc123")
	if err != nil {
		t.Fatalf("resolveMessageRef: unexpected error: %v", err)
	}
	if got.Text != "hello" {
		t.Errorf("resolveMessageRef(\"abc123\").Text = %q, want %q", got.Text, "hello")
	}
}

func TestResolveMessageRef_IndexOutOfRange(t *testing.T) {
	dir := withAppDataDir(t)
	ms := msgstore.New(dir)
	jid := "254700282181@s.whatsapp.net"
	_ = ms.Append(msgstore.Message{ID: "m1", ChatJID: jid, Text: "only one", Timestamp: 100})

	if _, err := resolveMessageRef(ms, jid, "5"); err == nil {
		t.Error("expected error for out-of-range index, got nil")
	}
}

func TestResolveMessageRef_UnknownID(t *testing.T) {
	dir := withAppDataDir(t)
	ms := msgstore.New(dir)
	jid := "254700282181@s.whatsapp.net"
	_ = ms.Append(msgstore.Message{ID: "m1", ChatJID: jid, Text: "only one", Timestamp: 100})

	if _, err := resolveMessageRef(ms, jid, "does-not-exist"); err == nil {
		t.Error("expected error for unknown message ID, got nil")
	}
}

// ---- recordSentMessage ----

func TestRecordSentMessage_AppendsWithFromMeTrue(t *testing.T) {
	dir := withAppDataDir(t)
	ms := msgstore.New(dir)
	jid, err := resolveJID("254700282181@s.whatsapp.net")
	if err != nil {
		t.Fatalf("resolveJID: %v", err)
	}

	recordSentMessage(ms, jid, "sent-msg-1", "hey there")

	msgs, err := ms.List(jid.String())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 stored message, got %d", len(msgs))
	}
	m := msgs[0]
	if !m.FromMe {
		t.Error("expected FromMe to be true")
	}
	if m.SenderJID != "me" {
		t.Errorf("SenderJID = %q, want %q", m.SenderJID, "me")
	}
	if m.Text != "hey there" {
		t.Errorf("Text = %q, want %q", m.Text, "hey there")
	}
	if m.RawProto == "" {
		t.Error("expected RawProto to be populated for later forwarding")
	}
}

func TestRecordSentMessage_NoopWithNilStoreOrEmptyID(t *testing.T) {
	withAppDataDir(t)
	jid, err := resolveJID("254700282181@s.whatsapp.net")
	if err != nil {
		t.Fatalf("resolveJID: %v", err)
	}

	// Should not panic with a nil store.
	recordSentMessage(nil, jid, "id", "text")

	dir := t.TempDir()
	ms := msgstore.New(dir)
	// Should not panic or append anything with an empty message ID.
	recordSentMessage(ms, jid, "", "text")
	msgs, err := ms.List(jid.String())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected no message stored for empty ID, got %d", len(msgs))
	}
}