package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/codebyoketch/wa-cli/internal/chatstore"
	waerrors "github.com/codebyoketch/wa-cli/internal/errors"
	"github.com/codebyoketch/wa-cli/internal/msgstore"
	"github.com/codebyoketch/wa-cli/internal/qr"
	"github.com/codebyoketch/wa-cli/internal/safety"
)

type Client struct {
	WA    *whatsmeow.Client
	log   waLog.Logger
	chats *chatstore.Store
	msgs  *msgstore.Store
}

// New builds a Client using the first (or a fresh, unpaired) device from
// container. chats/msgs may be nil for commands that don't need chat or
// message history (e.g. login/logout/status).
func New(ctx context.Context, container *sqlstore.Container, log waLog.Logger, chats *chatstore.Store, msgs *msgstore.Store) (*Client, error) {
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, waerrors.Wrap(err, "loading device store")
	}
	clientLog := waLog.Stdout("Client", "WARN", true)
	waClient := whatsmeow.NewClient(device, clientLog)
	c := &Client{WA: waClient, log: log, chats: chats, msgs: msgs}

	if chats != nil || msgs != nil {
		waClient.AddEventHandler(c.handleEvent)
	}

	return c, nil
}

func (c *Client) handleEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.HistorySync:
		if c.chats != nil {
			c.ingestHistorySync(v)
		}
	case *events.Message:
		c.ingestMessage(v)
	}
}

// ingestHistorySync populates chatstore from WhatsApp's initial (or
// on-demand) history sync payload, sent shortly after a fresh pairing or
// reconnect.
func (c *Client) ingestHistorySync(evt *events.HistorySync) {
	if evt.Data == nil {
		return
	}
	for _, conv := range evt.Data.GetConversations() {
		jid := conv.GetID()
		if jid == "" {
			continue
		}

		name := conv.GetName()
		msgs := conv.GetMessages()

		// Skip pure noise: no name and no message history at all. These
		// show up as bare @lid conversation stubs (e.g. group members
		// using WhatsApp's privacy-ID feature) that aren't real chats
		// worth surfacing in `wa chat list`.
		if name == "" && len(msgs) == 0 {
			continue
		}

		var lastAt int64
		var preview string
		if len(msgs) > 0 {
			last := msgs[len(msgs)-1].GetMessage()
			if ts := last.GetMessageTimestamp(); ts > 0 {
				lastAt = int64(ts) * 1000
			}
			preview = extractPreview(last.GetMessage())
		}

		err := c.chats.Upsert(chatstore.Chat{
			JID:                jid,
			Name:               name,
			IsGroup:            strings.HasSuffix(jid, "@g.us"),
			LastMessageAt:      lastAt,
			LastMessagePreview: preview,
			UnreadCount:        int(conv.GetUnreadCount()),
		})
		if err != nil {
			c.log.Warnf("chatstore upsert failed for %s: %v", jid, err)
		}
	}
}

// statusBroadcastJID is WhatsApp's pseudo-chat for Status/story updates.
// These aren't real conversations and shouldn't be printed or tracked as
// chats.
const statusBroadcastJID = "status@broadcast"

// ingestMessage keeps chatstore and msgstore current as new messages
// arrive/are sent after the initial sync (this is what wa watch leans on
// most heavily; wired in here too so a chat you're mid-conversation with
// during `wa chat list`/`wa chat open` still looks current).
func (c *Client) ingestMessage(evt *events.Message) {
	jid := evt.Info.Chat.String()
	if jid == statusBroadcastJID {
		return
	}

	preview := extractPreview(evt.Message)

	if c.chats != nil {
		err := c.chats.Upsert(chatstore.Chat{
			JID:                jid,
			Name:               evt.Info.PushName,
			IsGroup:            evt.Info.IsGroup,
			LastMessageAt:      evt.Info.Timestamp.UnixMilli(),
			LastMessagePreview: preview,
		})
		if err != nil {
			c.log.Warnf("chatstore upsert failed for %s: %v", jid, err)
		}

		if !evt.Info.IsFromMe {
			if err := c.chats.IncrementUnread(jid); err != nil {
				c.log.Warnf("chatstore unread increment failed for %s: %v", jid, err)
			}
		}
	}

	if c.msgs != nil {
		sender := evt.Info.Sender.String()
		if evt.Info.IsFromMe {
			sender = "me"
		}

		var raw string
		if evt.Message != nil {
			if b, err := proto.Marshal(evt.Message); err == nil {
				raw = base64.StdEncoding.EncodeToString(b)
			}
		}

		err := c.msgs.Append(msgstore.Message{
			ID:        evt.Info.ID,
			ChatJID:   jid,
			SenderJID: sender,
			Timestamp: evt.Info.Timestamp.UnixMilli(),
			Text:      preview,
			FromMe:    evt.Info.IsFromMe,
			RawProto:  raw,
		})
		if err != nil {
			c.log.Warnf("msgstore append failed for %s: %v", jid, err)
		}
	}
}

func extractPreview(msg interface{ GetConversation() string }) string {
	if msg == nil {
		return ""
	}
	text := msg.GetConversation()
	if len(text) > 80 {
		text = text[:80] + "…"
	}
	return text
}

// SyncChats connects, waits up to timeout for chat data to arrive via
// history sync / messages, then disconnects. Intended for one-shot
// commands like `wa chat list` that need reasonably current data without
// a long-running connection (that's what `wa watch`, Phase 5, is for).
func (c *Client) SyncChats(ctx context.Context, timeout time.Duration) error {
	if !c.IsLoggedIn() {
		return waerrors.ErrNotLoggedIn
	}
	if c.chats == nil {
		return waerrors.New("SyncChats called without a chatstore")
	}

	if err := c.WA.Connect(); err != nil {
		return waerrors.Wrap(err, "connecting to WhatsApp")
	}
	defer c.WA.Disconnect()

	select {
	case <-time.After(timeout):
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (c *Client) IsLoggedIn() bool {
	return c.WA.Store.ID != nil
}

// Connect establishes the WhatsApp connection. whatsmeow's WA.Connect()
// only *initiates* the handshake and returns immediately — a request
// sent right after can race a socket that isn't ready yet. Rather than a
// blind fixed sleep here (which was slow even when unnecessary), callers
// that immediately send a request should wrap that request in withRetry
// instead — genuinely instant when the connection happens to be ready,
// short backoff only when it actually isn't.
func (c *Client) Connect(ctx context.Context) error {
	if err := c.WA.Connect(); err != nil {
		return waerrors.Wrap(err, "connecting to WhatsApp")
	}
	return nil
}

// withRetry attempts fn up to 4 times with short, increasing backoff.
// Used for requests sent right after Connect(), where the first attempt
// may race a socket that isn't fully ready yet.
func withRetry(fn func() error) error {
	backoffs := []time.Duration{0, 300 * time.Millisecond, 800 * time.Millisecond, 2 * time.Second}
	var lastErr error
	for _, wait := range backoffs {
		if wait > 0 {
			time.Sleep(wait)
		}
		if err := fn(); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

// Login connects and, if unpaired, shows a QR code to scan. It stays
// connected for a grace period after pairing so WhatsApp has time to
// commit the linked device and, if a chatstore is attached, deliver the
// one-time HistorySync payload — disconnecting immediately after the QR
// "success" event is too early and the device never shows up in
// WhatsApp > Linked Devices.
func (c *Client) Login(ctx context.Context) error {
	if c.IsLoggedIn() {
		return c.Connect(ctx)
	}

	qrChan, err := c.WA.GetQRChannel(ctx)
	if err != nil {
		return waerrors.Wrap(err, "getting QR channel")
	}

	if err := c.WA.Connect(); err != nil {
		return waerrors.Wrap(err, "connecting to WhatsApp")
	}

	paired := false
	for evt := range qrChan {
		switch evt.Event {
		case "code":
			fmt.Println("Scan this QR code in WhatsApp: Settings > Linked Devices > Link a Device")
			qr.Print(evt.Code)
		case "success":
			paired = true
			fmt.Println("QR scanned — finishing setup, keep this open...")
		case "timeout":
			return waerrors.New("QR code expired, run 'wa login' again")
		default:
			c.log.Infof("login event: %s", evt.Event)
		}
	}

	if !paired {
		return waerrors.New("login did not complete")
	}

	// Give WhatsApp time to finish the post-pairing handshake and, if a
	// chatstore is attached, deliver the one-time HistorySync payload.
	// This is a fixed wait rather than waiting on *events.Connected —
	// see Connect()'s comment for why.
	wait := 15 * time.Second
	if c.chats != nil {
		wait = 25 * time.Second
		fmt.Println("Syncing chat history — this can take a bit on first login...")
	} else {
		fmt.Println("Finishing setup...")
	}

	select {
	case <-time.After(wait):
	case <-ctx.Done():
		return ctx.Err()
	}

	fmt.Println("Login successful.")
	return nil
}

// Watch keeps a long-running connection open, printing incoming messages
// and reconnecting on drops, until ctx is cancelled (e.g. Ctrl+C).
//
// Given the connection instability observed during Phase 2/3 testing
// (frequent "Error sending close to websocket" resets, most likely
// CGNAT-related on some networks), this is deliberately more defensive
// than SyncChats: it doesn't just trust whatsmeow's built-in reconnect
// (which may or may not be enabled depending on your pinned version —
// check `go doc go.mau.fi/whatsmeow.Client` for an EnableAutoReconnect
// field or similar if reconnects don't happen automatically) — it also
// watches for *events.Disconnected itself and retries with backoff.
func (c *Client) Watch(ctx context.Context, guard *safety.Guard) error {
	if !c.IsLoggedIn() {
		return waerrors.ErrNotLoggedIn
	}

	c.WA.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			c.printIncoming(v, guard)
		case *events.Connected:
			fmt.Println("[connected]")
		case *events.Disconnected:
			fmt.Println("[disconnected — attempting to reconnect...]")
			go c.reconnectWithBackoff(ctx)
		case *events.LoggedOut:
			fmt.Println("[logged out remotely — run 'wa login' again]")
		}
	})

	if err := c.WA.Connect(); err != nil {
		return waerrors.Wrap(err, "connecting to WhatsApp")
	}
	defer c.WA.Disconnect()

	fmt.Println("Watching for new messages. Press Ctrl+C to stop.")

	<-ctx.Done()
	fmt.Println("\nStopping...")
	return nil
}

// reconnectWithBackoff retries WA.Connect() with exponential backoff
// (capped at 30s) until it succeeds or ctx is cancelled. This is a
// belt-and-suspenders fallback alongside whatsmeow's own reconnect
// handling — worth keeping given how unreliable the connection proved
// to be in earlier testing on some networks.
func (c *Client) reconnectWithBackoff(ctx context.Context) {
	backoff := 2 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		if err := c.WA.Connect(); err != nil {
			c.log.Warnf("reconnect attempt failed: %v", err)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		fmt.Println("[reconnected]")
		return
	}
}

// printIncoming prints one incoming message and marks its sender known
// (so a later `wa chat send` reply doesn't trigger the new-recipient
// confirmation prompt).
func (c *Client) printIncoming(evt *events.Message, guard *safety.Guard) {
	if evt.Info.IsFromMe || evt.Info.Chat.String() == statusBroadcastJID {
		return
	}

	sender := evt.Info.PushName
	if sender == "" {
		sender = evt.Info.Sender.User
	}

	text := extractPreview(evt.Message)
	if text == "" {
		text = "[non-text message]"
	}

	ts := evt.Info.Timestamp.Local().Format("15:04:05")
	fmt.Printf("[%s] %s (%s): %s\n", ts, sender, evt.Info.Chat.String(), text)

	if guard != nil {
		if err := guard.MarkKnown(evt.Info.Sender.String()); err != nil {
			c.log.Warnf("failed to mark sender known: %v", err)
		}
	}
}

func (c *Client) Logout(ctx context.Context) error {
	if !c.IsLoggedIn() {
		return waerrors.ErrNotLoggedIn
	}
	err := withRetry(func() error {
		return c.WA.Logout(ctx)
	})
	if err != nil {
		return waerrors.Wrap(err, "logging out")
	}
	return nil
}

func (c *Client) Disconnect() {
	c.WA.Disconnect()
}

// SendText sends a plain text message to jid and returns the sent
// message's ID (for later reply/forward reference). Callers should call
// Connect() first.
func (c *Client) SendText(ctx context.Context, jid types.JID, text string) (string, error) {
	msg := &waProto.Message{
		Conversation: proto.String(text),
	}
	var resp whatsmeow.SendResponse
	err := withRetry(func() error {
		var sendErr error
		resp, sendErr = c.WA.SendMessage(ctx, jid, msg)
		return sendErr
	})
	if err != nil {
		return "", waerrors.Wrap(err, "sending message")
	}
	return resp.ID, nil
}

// SendReply sends text to jid as a quoted reply to quoted. If quoted's
// original content couldn't be reconstructed (e.g. an old or unsupported
// message type), the reply is still sent, just without the visible quote.
func (c *Client) SendReply(ctx context.Context, jid types.JID, text string, quoted msgstore.Message) (string, error) {
	ctxInfo := &waProto.ContextInfo{
		StanzaID:    proto.String(quoted.ID),
		Participant: proto.String(quoted.SenderJID),
	}
	if quotedMsg := decodeRawProto(quoted.RawProto); quotedMsg != nil {
		ctxInfo.QuotedMessage = quotedMsg
	}

	msg := &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text:        proto.String(text),
			ContextInfo: ctxInfo,
		},
	}
	var resp whatsmeow.SendResponse
	err := withRetry(func() error {
		var sendErr error
		resp, sendErr = c.WA.SendMessage(ctx, jid, msg)
		return sendErr
	})
	if err != nil {
		return "", waerrors.Wrap(err, "sending reply")
	}
	return resp.ID, nil
}

// ForwardMessage re-sends quoted's content to toJID, marked as forwarded.
// Only plain text and extended-text (e.g. text with a link preview)
// messages are supported in this pass — media forwarding would need
// re-uploading the media, which is out of scope here.
func (c *Client) ForwardMessage(ctx context.Context, toJID types.JID, quoted msgstore.Message) (string, error) {
	original := decodeRawProto(quoted.RawProto)
	if original == nil {
		return "", waerrors.New("original message content isn't available to forward (too old, or an unsupported message type)")
	}

	var msg *waProto.Message
	switch {
	case original.GetConversation() != "":
		msg = &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text: proto.String(original.GetConversation()),
				ContextInfo: &waProto.ContextInfo{
					IsForwarded:     proto.Bool(true),
					ForwardingScore: proto.Uint32(1),
				},
			},
		}
	case original.GetExtendedTextMessage() != nil:
		msg = original
		ext := msg.GetExtendedTextMessage()
		if ext.ContextInfo == nil {
			ext.ContextInfo = &waProto.ContextInfo{}
		}
		ext.ContextInfo.IsForwarded = proto.Bool(true)
		ext.ContextInfo.ForwardingScore = proto.Uint32(ext.ContextInfo.GetForwardingScore() + 1)
	default:
		return "", waerrors.New("forwarding this message type isn't supported yet (text only)")
	}

	var resp whatsmeow.SendResponse
	err := withRetry(func() error {
		var sendErr error
		resp, sendErr = c.WA.SendMessage(ctx, toJID, msg)
		return sendErr
	})
	if err != nil {
		return "", waerrors.Wrap(err, "forwarding message")
	}
	return resp.ID, nil
}

// decodeRawProto reconstructs a stored message's original content from
// its base64-encoded protobuf, returning nil if unavailable or decoding
// fails (callers should degrade gracefully rather than error out — a
// reply/forward without the original quote attached is still useful).
func decodeRawProto(raw string) *waProto.Message {
	if raw == "" {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}
	msg := &waProto.Message{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil
	}
	return msg
}
