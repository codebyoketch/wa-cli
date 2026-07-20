package cmd

// NOTE: written without a working Go 1.25 toolchain in the authoring
// sandbox (see internal/whatsapp/client_test.go's header comment for
// why) — syntax-checked with gofmt only. Please run
// `go test ./cmd/... -run 'TestIsAllDigits|TestResolve|TestRecordSentMessage' -v` locally.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"bytes"
	"strings"
	"log/slog"
	"testing"

	"github.com/codebyoketch/wa-cli/internal/app"
	"github.com/codebyoketch/wa-cli/internal/chatstore"
	"github.com/codebyoketch/wa-cli/internal/config"
	"github.com/codebyoketch/wa-cli/internal/msgstore"
)

// NOTE: written without a working Go 1.25 toolchain in the authoring
// sandbox (see internal/whatsapp/client_test.go's header comment for
// why) — syntax-checked with gofmt only. Please run
// `go test ./cmd/... -run TestCaptureLibraryStdout -v` locally.

func TestCaptureLibraryStdout_RedirectsDuringCall(t *testing.T) {
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	// Substitute os.Stderr for the duration of the test so we can
	// observe what captureLibraryStdout redirects os.Stdout *to*,
	// without polluting the real test output.
	origStderr := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origStderr })

	var sawDuringCall *os.File
	callErr := captureLibraryStdout(func() error {
		sawDuringCall = os.Stdout
		fmt.Println("this should land on stderr, not stdout")
		return nil
	})
	if callErr != nil {
		t.Fatalf("captureLibraryStdout: unexpected error: %v", callErr)
	}

	if sawDuringCall != os.Stderr {
		t.Error("expected os.Stdout to be redirected to os.Stderr during fn")
	}
	if os.Stdout != origStdout {
		t.Error("expected os.Stdout to be restored after captureLibraryStdout returns")
	}

	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading captured output: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected fn's println to have landed on the redirected stream")
	}
}

func TestCaptureLibraryStdout_RestoresStdoutEvenOnError(t *testing.T) {
	origStdout := os.Stdout
	wantErr := errors.New("boom")

	gotErr := captureLibraryStdout(func() error {
		return wantErr
	})

	if !errors.Is(gotErr, wantErr) {
		t.Errorf("captureLibraryStdout error = %v, want %v", gotErr, wantErr)
	}
	if os.Stdout != origStdout {
		t.Error("expected os.Stdout to be restored even when fn returns an error")
	}
}

// NOTE: written without a working Go 1.25 toolchain in the authoring
// sandbox (see internal/whatsapp/client_test.go's header comment for
// why) — syntax-checked with gofmt only. Please run
// `go test ./cmd/... -run TestVersionCmd -v` locally.

func TestVersionCmd_PrintsVersionString(t *testing.T) {
	c := newTestCmd()
	var buf bytes.Buffer
	c.SetOut(&buf)

	if err := versionCmd.RunE(c, nil); err != nil {
		t.Fatalf("versionCmd.RunE: unexpected error: %v", err)
	}

	if strings.TrimSpace(buf.String()) == "" {
		t.Error("expected non-empty version output")
	}
}

// captureStdout runs fn and returns whatever it wrote to os.Stdout.
// Needed here because the completion RunE funcs write straight to
// os.Stdout (via cobra's Gen*Completion helpers) rather than
// cmd.OutOrStdout(), so SetOut(buf) alone wouldn't capture anything.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	fnErr := fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe writer: %v", err)
	}
	os.Stdout = orig

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}
	return string(out), fnErr
}

func TestCompletionCmd_Bash(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return completionCmd.RunE(completionCmd, []string{"bash"})
	})
	if err != nil {
		t.Fatalf("completion bash: unexpected error: %v", err)
	}
	if !strings.Contains(out, "bash completion") && !strings.Contains(out, "complete") {
		t.Errorf("expected bash-completion-looking output, got %d bytes starting %q", len(out), firstN(out, 60))
	}
}

func TestCompletionCmd_Zsh(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return completionCmd.RunE(completionCmd, []string{"zsh"})
	})
	if err != nil {
		t.Fatalf("completion zsh: unexpected error: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected non-empty zsh completion output")
	}
}

func TestCompletionCmd_Fish(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return completionCmd.RunE(completionCmd, []string{"fish"})
	})
	if err != nil {
		t.Fatalf("completion fish: unexpected error: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected non-empty fish completion output")
	}
}

func TestCompletionCmd_PowerShell(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return completionCmd.RunE(completionCmd, []string{"powershell"})
	})
	if err != nil {
		t.Fatalf("completion powershell: unexpected error: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected non-empty powershell completion output")
	}
}

func TestCompletionCmd_RejectsUnknownShell(t *testing.T) {
	err := completionCmd.Args(completionCmd, []string{"tcsh"})
	if err == nil {
		t.Error("expected an unknown shell name to be rejected by Args validation")
	}
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
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