package whatsapp

// NOTE: These tests were written without the ability to compile/run them in
// the authoring sandbox — go.mod requires go 1.25.0 and the toolchain
// auto-fetch needed to get it (proxy.golang.org) was blocked by network
// egress rules there. Please run `go test ./internal/whatsapp/...` locally;
// if any whatsmeow type/field name below doesn't match the pinned version
// in go.sum, those are the first things to check.

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/codebyoketch/wa-cli/internal/chatstore"
	"github.com/codebyoketch/wa-cli/internal/msgstore"
)

func testLogger() waLog.Logger {
	return waLog.Stdout("test", "ERROR", false)
}

// ---- isPseudoChat ----

func TestIsPseudoChat(t *testing.T) {
	cases := []struct {
		name string
		jid  string
		want bool
	}{
		{"status broadcast", "status@broadcast", true},
		{"newsletter", "120363012345678901@newsletter", true},
		{"normal contact", "12345@s.whatsapp.net", false},
		{"group chat", "12345-67890@g.us", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPseudoChat(tc.jid); got != tc.want {
				t.Errorf("isPseudoChat(%q) = %v, want %v", tc.jid, got, tc.want)
			}
		})
	}
}

// ---- extractPreview ----

type fakeConversation struct{ text string }

func (f fakeConversation) GetConversation() string { return f.text }

func TestExtractPreview(t *testing.T) {
	if got := extractPreview(nil); got != "" {
		t.Errorf("nil message: got %q, want empty", got)
	}

	if got := extractPreview(fakeConversation{"hello there"}); got != "hello there" {
		t.Errorf("short text: got %q, want %q", got, "hello there")
	}

	long := strings.Repeat("a", 100)
	got := extractPreview(fakeConversation{long})
	want := strings.Repeat("a", 80) + "…"
	if got != want {
		t.Errorf("long text not truncated correctly: got %d runes, want %d", len([]rune(got)), len([]rune(want)))
	}
}

// ---- classifyMessage ----

func TestClassifyMessage(t *testing.T) {
	if mt, text := classifyMessage(nil); mt != "" || text != "" {
		t.Errorf("nil message: got (%q, %q), want (\"\", \"\")", mt, text)
	}

	msg := &waProto.Message{Conversation: proto.String("plain text")}
	if mt, text := classifyMessage(msg); mt != "" || text != "plain text" {
		t.Errorf("conversation: got (%q, %q), want (\"\", %q)", mt, text, "plain text")
	}

	msg = &waProto.Message{ExtendedTextMessage: &waProto.ExtendedTextMessage{Text: proto.String("extended")}}
	if mt, text := classifyMessage(msg); mt != "" || text != "extended" {
		t.Errorf("extended text: got (%q, %q), want (\"\", %q)", mt, text, "extended")
	}

	msg = &waProto.Message{ImageMessage: &waProto.ImageMessage{Caption: proto.String("a photo")}}
	if mt, text := classifyMessage(msg); mt != "image" || text != "a photo" {
		t.Errorf("image: got (%q, %q), want (\"image\", %q)", mt, text, "a photo")
	}

	msg = &waProto.Message{VideoMessage: &waProto.VideoMessage{Caption: proto.String("a clip")}}
	if mt, text := classifyMessage(msg); mt != "video" || text != "a clip" {
		t.Errorf("video: got (%q, %q), want (\"video\", %q)", mt, text, "a clip")
	}

	msg = &waProto.Message{AudioMessage: &waProto.AudioMessage{PTT: proto.Bool(true)}}
	if mt, text := classifyMessage(msg); mt != "voice note" || text != "" {
		t.Errorf("voice note: got (%q, %q), want (\"voice note\", \"\")", mt, text)
	}

	msg = &waProto.Message{AudioMessage: &waProto.AudioMessage{PTT: proto.Bool(false)}}
	if mt, text := classifyMessage(msg); mt != "audio" || text != "" {
		t.Errorf("audio: got (%q, %q), want (\"audio\", \"\")", mt, text)
	}

	msg = &waProto.Message{DocumentMessage: &waProto.DocumentMessage{Caption: proto.String("a file")}}
	if mt, text := classifyMessage(msg); mt != "document" || text != "a file" {
		t.Errorf("document: got (%q, %q), want (\"document\", %q)", mt, text, "a file")
	}

	msg = &waProto.Message{StickerMessage: &waProto.StickerMessage{}}
	if mt, text := classifyMessage(msg); mt != "sticker" || text != "" {
		t.Errorf("sticker: got (%q, %q), want (\"sticker\", \"\")", mt, text)
	}

	msg = &waProto.Message{}
	if mt, text := classifyMessage(msg); mt != "" || text != "" {
		t.Errorf("unrecognized/empty message: got (%q, %q), want (\"\", \"\")", mt, text)
	}
}

// ---- bestContactName ----

func TestBestContactName(t *testing.T) {
	cases := []struct {
		name string
		info types.ContactInfo
		want string
	}{
		{"prefers full name", types.ContactInfo{FullName: "Alice Wonderland", FirstName: "Alice", PushName: "Ali", BusinessName: "Wonderland Inc"}, "Alice Wonderland"},
		{"falls back to first name", types.ContactInfo{FirstName: "Bob", PushName: "Bobby", BusinessName: "Bob's Shop"}, "Bob"},
		{"falls back to push name", types.ContactInfo{PushName: "Charlie", BusinessName: "Charlie's Cafe"}, "Charlie"},
		{"falls back to business name", types.ContactInfo{BusinessName: "Dave's Diner"}, "Dave's Diner"},
		{"empty when nothing set", types.ContactInfo{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bestContactName(tc.info); got != tc.want {
				t.Errorf("bestContactName(%+v) = %q, want %q", tc.info, got, tc.want)
			}
		})
	}
}

// ---- withRetry ----

func TestWithRetry_SucceedsFirstTry(t *testing.T) {
	calls := 0
	err := withRetry(func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("withRetry: unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestWithRetry_SucceedsAfterFailures(t *testing.T) {
	calls := 0
	err := withRetry(func() error {
		calls++
		if calls < 3 {
			return errString("not ready yet")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withRetry: unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestWithRetry_ExhaustsAndReturnsLastError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow full-backoff test in -short mode")
	}
	calls := 0
	err := withRetry(func() error {
		calls++
		return errString("always fails")
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if calls != 4 {
		t.Errorf("expected 4 attempts, got %d", calls)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

// ---- markForwarded ----

func TestMarkForwarded_PlainText(t *testing.T) {
	original := &waProto.Message{Conversation: proto.String("hi there")}
	got, err := markForwarded(original)
	if err != nil {
		t.Fatalf("markForwarded: unexpected error: %v", err)
	}
	if got.GetExtendedTextMessage() == nil {
		t.Fatal("expected plain text to be normalized to ExtendedTextMessage")
	}
	if got.GetExtendedTextMessage().GetText() != "hi there" {
		t.Errorf("text = %q, want %q", got.GetExtendedTextMessage().GetText(), "hi there")
	}
	ci := got.GetExtendedTextMessage().GetContextInfo()
	if !ci.GetIsForwarded() || ci.GetForwardingScore() != 1 {
		t.Errorf("unexpected ContextInfo: forwarded=%v score=%d", ci.GetIsForwarded(), ci.GetForwardingScore())
	}
}

func TestMarkForwarded_BumpsExistingScore(t *testing.T) {
	original := &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String("already forwarded once"),
			ContextInfo: &waProto.ContextInfo{
				IsForwarded:     proto.Bool(true),
				ForwardingScore: proto.Uint32(1),
			},
		},
	}
	got, err := markForwarded(original)
	if err != nil {
		t.Fatalf("markForwarded: unexpected error: %v", err)
	}
	if score := got.GetExtendedTextMessage().GetContextInfo().GetForwardingScore(); score != 2 {
		t.Errorf("ForwardingScore = %d, want 2", score)
	}
}

func TestMarkForwarded_MediaTypes(t *testing.T) {
	t.Run("image", func(t *testing.T) {
		original := &waProto.Message{ImageMessage: &waProto.ImageMessage{Caption: proto.String("pic")}}
		got, err := markForwarded(original)
		if err != nil || got.GetImageMessage() == nil {
			t.Fatalf("markForwarded(image): got=%v err=%v", got, err)
		}
		if !got.GetImageMessage().GetContextInfo().GetIsForwarded() {
			t.Error("expected IsForwarded to be set on image message")
		}
	})

	t.Run("sticker", func(t *testing.T) {
		original := &waProto.Message{StickerMessage: &waProto.StickerMessage{}}
		got, err := markForwarded(original)
		if err != nil || got.GetStickerMessage() == nil {
			t.Fatalf("markForwarded(sticker): got=%v err=%v", got, err)
		}
	})
}

func TestMarkForwarded_UnsupportedType(t *testing.T) {
	// A message with none of the recognized fields set (e.g. a reaction
	// or poll) should be rejected rather than silently forwarded as
	// something else.
	original := &waProto.Message{}
	_, err := markForwarded(original)
	if err == nil {
		t.Fatal("expected error for unsupported message type, got nil")
	}
}

// ---- decodeRawProto ----

func TestDecodeRawProto_Empty(t *testing.T) {
	if got := decodeRawProto(""); got != nil {
		t.Errorf("expected nil for empty input, got %+v", got)
	}
}

func TestDecodeRawProto_InvalidBase64(t *testing.T) {
	if got := decodeRawProto("not valid base64!!!"); got != nil {
		t.Errorf("expected nil for invalid base64, got %+v", got)
	}
}

func TestDecodeRawProto_RoundTrip(t *testing.T) {
	original := &waProto.Message{Conversation: proto.String("round trip me")}
	b, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(b)

	got := decodeRawProto(encoded)
	if got == nil {
		t.Fatal("expected decoded message, got nil")
	}
	if got.GetConversation() != "round trip me" {
		t.Errorf("GetConversation() = %q, want %q", got.GetConversation(), "round trip me")
	}
}

// ---- ingestMessage ----

func newTestClient(t *testing.T, withChats, withMsgs bool) *Client {
	t.Helper()
	c := &Client{log: testLogger()}
	if withChats {
		c.chats = chatstore.New(t.TempDir())
	}
	if withMsgs {
		c.msgs = msgstore.New(t.TempDir())
	}
	return c
}

func testJID(user, server string) types.JID {
	return types.NewJID(user, server)
}

func TestIngestMessage_SkipsPseudoChat(t *testing.T) {
	c := newTestClient(t, true, true)
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat: testJID("status", "broadcast"),
			},
			ID: "1",
		},
		Message: &waProto.Message{Conversation: proto.String("hi")},
	}
	c.ingestMessage(evt)

	msgs, err := c.msgs.List("status@broadcast")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected pseudo-chat message to be skipped, got %d messages", len(msgs))
	}
}

func TestIngestMessage_IncomingDM_SetsNameAndIncrementsUnread(t *testing.T) {
	c := newTestClient(t, true, true)
	chatJID := testJID("12345", types.DefaultUserServer)
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chatJID,
				Sender:   chatJID,
				IsFromMe: false,
				IsGroup:  false,
			},
			ID:        "MSG1",
			PushName:  "Alice",
			Timestamp: time.Now(),
		},
		Message: &waProto.Message{Conversation: proto.String("hello!")},
	}
	c.ingestMessage(evt)

	chat, ok, err := c.chats.Get(chatJID.String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("expected chat to be created")
	}
	if chat.Name != "Alice" {
		t.Errorf("chat.Name = %q, want %q", chat.Name, "Alice")
	}
	if chat.UnreadCount != 1 {
		t.Errorf("chat.UnreadCount = %d, want 1", chat.UnreadCount)
	}

	msgs, err := c.msgs.List(chatJID.String())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Text != "hello!" {
		t.Fatalf("unexpected stored messages: %+v", msgs)
	}
	if msgs[0].SenderJID != chatJID.String() {
		t.Errorf("SenderJID = %q, want %q", msgs[0].SenderJID, chatJID.String())
	}
}

func TestIngestMessage_OutgoingDM_DoesNotFlipNameOrIncrementUnread(t *testing.T) {
	c := newTestClient(t, true, true)
	chatJID := testJID("12345", types.DefaultUserServer)

	// Seed the chat with the other person's name, as an incoming message
	// would have set it.
	if err := c.chats.Upsert(chatstore.Chat{JID: chatJID.String(), Name: "Alice"}); err != nil {
		t.Fatalf("seed Upsert: %v", err)
	}

	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chatJID,
				Sender:   testJID("me", types.DefaultUserServer),
				IsFromMe: true,
				IsGroup:  false,
			},
			ID:        "MSG2",
			PushName:  "MyOwnProfileName",
			Timestamp: time.Now(),
		},
		Message: &waProto.Message{Conversation: proto.String("outgoing reply")},
	}
	c.ingestMessage(evt)

	chat, ok, err := c.chats.Get(chatJID.String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("expected chat to still exist")
	}
	if chat.Name != "Alice" {
		t.Errorf("chat.Name = %q, want unchanged %q (own outgoing message shouldn't flip it)", chat.Name, "Alice")
	}
	if chat.UnreadCount != 0 {
		t.Errorf("chat.UnreadCount = %d, want 0 for own outgoing message", chat.UnreadCount)
	}

	msgs, err := c.msgs.List(chatJID.String())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 1 || msgs[0].SenderJID != "me" {
		t.Fatalf("expected outgoing message to be stored with SenderJID=me, got %+v", msgs)
	}
	if !msgs[0].FromMe {
		t.Error("expected FromMe to be true")
	}
}

func TestIngestMessage_GroupMessage_DoesNotSetNameFromSender(t *testing.T) {
	c := newTestClient(t, true, true)
	groupJID := testJID("12345-67890", types.GroupServer)
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     groupJID,
				Sender:   testJID("11111", types.DefaultUserServer),
				IsFromMe: false,
				IsGroup:  true,
			},
			ID:        "MSG3",
			PushName:  "SomeGroupMember",
			Timestamp: time.Now(),
		},
		Message: &waProto.Message{Conversation: proto.String("group hi")},
	}
	c.ingestMessage(evt)

	chat, ok, err := c.chats.Get(groupJID.String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("expected group chat to be created")
	}
	if chat.Name != "" {
		t.Errorf("chat.Name = %q, want empty (group name shouldn't come from a member's PushName)", chat.Name)
	}
	if !chat.IsGroup {
		t.Error("expected IsGroup to be true")
	}
}

func TestIngestMessage_CallsOnIncomingCallback(t *testing.T) {
	c := newTestClient(t, true, true)
	var received *msgstore.Message
	c.OnIncomingMessage(func(m msgstore.Message) {
		received = &m
	})

	chatJID := testJID("12345", types.DefaultUserServer)
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chatJID, Sender: chatJID},
			ID:            "MSG4",
			Timestamp:     time.Now(),
		},
		Message: &waProto.Message{Conversation: proto.String("callback test")},
	}
	c.ingestMessage(evt)

	if received == nil {
		t.Fatal("expected OnIncomingMessage callback to fire")
	}
	if received.Text != "callback test" {
		t.Errorf("callback message text = %q, want %q", received.Text, "callback test")
	}
}

func TestIngestMessage_NilStoresDoNotPanic(t *testing.T) {
	c := newTestClient(t, false, false)
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: testJID("12345", types.DefaultUserServer)},
			ID:            "MSG5",
			Timestamp:     time.Now(),
		},
		Message: &waProto.Message{Conversation: proto.String("no stores attached")},
	}
	c.ingestMessage(evt) // should not panic with nil chats/msgs
}