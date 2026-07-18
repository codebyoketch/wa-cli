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

// Login connects and, if unpaired, shows a QR code to scan. It stays
// connected until the post-pairing sync actually completes (or times out),
// so the phone has time to commit the linked device — disconnecting right
// after the QR "success" event is too early and the device never shows up
// in WhatsApp > Linked Devices.
func (c *Client) Login(ctx context.Context) error {
	// Fires once the full connection (including post-pairing sync) is ready.
	connected := make(chan struct{}, 1)
	c.WA.AddEventHandler(func(evt interface{}) {
		switch evt.(type) {
		case *events.Connected:
			select {
			case connected <- struct{}{}:
			default:
			}
		}
	})

	if c.IsLoggedIn() {
		if err := c.WA.Connect(); err != nil {
			return waerrors.Wrap(err, "connecting to WhatsApp")
		}
		<-connected
		return nil
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

	// Wait for the actual post-pairing sync to finish, capped so we don't
	// hang forever if something's wrong.
	select {
	case <-connected:
		fmt.Println("Login successful.")
	case <-time.After(20 * time.Second):
		fmt.Println("Paired, but sync is taking a while — check Linked Devices on your phone. If it's not there, run 'wa logout' then 'wa login' again.")
	}

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
