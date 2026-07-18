package whatsapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/codebyoketch/wa-cli/internal/chatstore"
	waerrors "github.com/codebyoketch/wa-cli/internal/errors"
	"github.com/codebyoketch/wa-cli/internal/qr"
)

type Client struct {
	WA    *whatsmeow.Client
	log   waLog.Logger
	chats *chatstore.Store
}

// New builds a Client using the first (or a fresh, unpaired) device from
// container. chats may be nil if the caller doesn't need chat syncing
// (e.g. login/logout/status don't).
func New(ctx context.Context, container *sqlstore.Container, log waLog.Logger, chats *chatstore.Store) (*Client, error) {
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, waerrors.Wrap(err, "loading device store")
	}
	clientLog := waLog.Stdout("Client", "WARN", true)
	waClient := whatsmeow.NewClient(device, clientLog)
	c := &Client{WA: waClient, log: log, chats: chats}

	if chats != nil {
		waClient.AddEventHandler(c.handleEvent)
	}

	return c, nil
}

func (c *Client) handleEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.HistorySync:
		c.ingestHistorySync(v)
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
		var lastAt int64
		var preview string
		msgs := conv.GetMessages()
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

// ingestMessage keeps chatstore current as new messages arrive/are sent
// after the initial sync (this is what Phase 5's `wa watch` will lean on
// more heavily; wired in here too so a chat you're mid-conversation with
// during `wa chat list` still looks current).
func (c *Client) ingestMessage(evt *events.Message) {
	jid := evt.Info.Chat.String()
	preview := extractPreview(evt.Message)

	err := c.chats.Upsert(chatstore.Chat{
		JID:                jid,
		IsGroup:            evt.Info.IsGroup,
		LastMessageAt:      evt.Info.Timestamp.UnixMilli(),
		LastMessagePreview: preview,
	})
	if err != nil {
		c.log.Warnf("chatstore upsert failed for %s: %v", jid, err)
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

// Connect establishes the WhatsApp connection and gives it a moment to
// actually finish before returning. whatsmeow's WA.Connect() only
// *initiates* the handshake and returns immediately — sending a request
// right after races a socket that isn't ready yet. We deliberately don't
// gate this on *events.Connected: in testing it hasn't fired reliably
// within any reasonable window, even on connections that demonstrably
// succeeded (confirmed via `wa status` immediately after).
func (c *Client) Connect(ctx context.Context) error {
	if err := c.WA.Connect(); err != nil {
		return waerrors.Wrap(err, "connecting to WhatsApp")
	}

	select {
	case <-time.After(5 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
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

func (c *Client) Logout(ctx context.Context) error {
	if !c.IsLoggedIn() {
		return waerrors.ErrNotLoggedIn
	}
	if err := c.WA.Logout(ctx); err != nil {
		return waerrors.Wrap(err, "logging out")
	}
	return nil
}

func (c *Client) Disconnect() {
	c.WA.Disconnect()
}
