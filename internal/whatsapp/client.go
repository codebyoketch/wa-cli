// Package whatsapp wraps go.mau.fi/whatsmeow with a smaller,
// wa-cli-shaped API (Client.ListChats, SendImage, GroupInfo, and so on)
// rather than leaking whatsmeow's types everywhere. This is the one
// package that talks to WhatsApp's servers; everything under cmd/ goes
// through it rather than importing whatsmeow directly.
//
// Client wires together three separate local stores (see
// internal/store, internal/chatstore, internal/msgstore) plus optional
// desktop notifications (internal/notify) — see ARCHITECTURE.md for how
// they fit together and why they're kept separate.
package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
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
	"github.com/codebyoketch/wa-cli/internal/notify"
	"github.com/codebyoketch/wa-cli/internal/qr"
	"github.com/codebyoketch/wa-cli/internal/safety"
)

// Client is wa-cli's WhatsApp client: a thin, higher-level wrapper
// around a *whatsmeow.Client (exported as WA) plus the local
// chatstore/msgstore state and notification settings needed to serve
// wa-cli's commands and TUI. Construct one with New.
type Client struct {
	WA         *whatsmeow.Client
	log        waLog.Logger
	chats      *chatstore.Store
	msgs       *msgstore.Store
	onIncoming func(msgstore.Message)

	// Desktop notifications are opt-in (default off) — only wa watch
	// and the TUI call SetNotifications to enable them. Without this,
	// any brief background sync that also passes chats/msgs to New()
	// (e.g. `wa chat list`, `wa chat send`) would start popping
	// notifications too, which isn't what those commands are for.
	notifyEnabled     bool
	notifyGroups      bool
	notifyShowPreview bool
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

// OnIncomingMessage registers fn to be called for every incoming message,
// after it's been ingested into chatstore/msgstore. Lets callers with
// their own event loop (like the TUI) bridge live updates in without
// duplicating ingestion logic. Only one callback is supported; a second
// call replaces the first.
func (c *Client) OnIncomingMessage(fn func(msgstore.Message)) {
	c.onIncoming = fn
}

// SetNotifications turns on desktop notifications for incoming messages.
// Off by default — see the Client struct's notify* field comments for
// why this needs to be explicit rather than inferred from chats/msgs
// being present.
func (c *Client) SetNotifications(enabled, groups, showPreview bool) {
	c.notifyEnabled = enabled
	c.notifyGroups = groups
	c.notifyShowPreview = showPreview
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
		if jid == "" || isPseudoChat(jid) {
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
const statusBroadcastJID = "status@broadcast"

// newsletterSuffix marks WhatsApp Channel subscriptions (e.g.
// "120363...@newsletter"), unlike status@broadcast these don't have one
// fixed JID, so they're matched by suffix instead of exact equality.
const newsletterSuffix = "@newsletter"

// isPseudoChat reports whether jid is a non-conversation pseudo-chat
// (Status broadcasts, Channel/newsletter subscriptions) that shouldn't be
// printed or tracked as a real chat.
func isPseudoChat(jid string) bool {
	return jid == statusBroadcastJID || strings.HasSuffix(jid, newsletterSuffix)
}

// ingestMessage keeps chatstore and msgstore current as new messages
// arrive/are sent after the initial sync (this is what wa watch leans on
// most heavily; wired in here too so a chat you're mid-conversation with
// during `wa chat list`/`wa chat open` still looks current).
func (c *Client) ingestMessage(evt *events.Message) {
	jid := evt.Info.Chat.String()
	if isPseudoChat(jid) {
		return
	}

	mediaType, text := classifyMessage(evt.Message)
	preview := text
	switch {
	case mediaType != "" && text != "":
		preview = fmt.Sprintf("[%s] %s", mediaType, text)
	case mediaType != "":
		preview = fmt.Sprintf("[%s]", mediaType)
	}

	if c.chats != nil {
		update := chatstore.Chat{
			JID:                jid,
			IsGroup:            evt.Info.IsGroup,
			LastMessageAt:      evt.Info.Timestamp.UnixMilli(),
			LastMessagePreview: preview,
		}
		// PushName is whoever actually sent this specific message —
		// correct as the chat's name for a 1:1 chat, but only when it's
		// the *other* person's message. Your own outgoing messages also
		// fire this event with PushName set to your own profile name,
		// which would otherwise flip the stored chat name back and
		// forth between yours and theirs depending on who sent last.
		// Also wrong for groups (would overwrite the group's real name
		// with whichever member happened to send most recently).
		if !evt.Info.IsGroup && !evt.Info.IsFromMe {
			update.Name = evt.Info.PushName
		}
		err := c.chats.Upsert(update)
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

		stored := msgstore.Message{
			ID:        evt.Info.ID,
			ChatJID:   jid,
			SenderJID: sender,
			Timestamp: evt.Info.Timestamp.UnixMilli(),
			Text:      text,
			FromMe:    evt.Info.IsFromMe,
			MediaType: mediaType,
			RawProto:  raw,
		}

		if err := c.msgs.Append(stored); err != nil {
			c.log.Warnf("msgstore append failed for %s: %v", jid, err)
		} else if c.onIncoming != nil {
			c.onIncoming(stored)
		}
	}

	if !evt.Info.IsFromMe && c.notifyEnabled {
		shouldNotify := c.notifyGroups || !evt.Info.IsGroup
		if shouldNotify && c.chats != nil {
			if chat, ok, err := c.chats.Get(jid); err == nil && ok && chat.Muted {
				shouldNotify = false
			}
		}
		if shouldNotify {
			title := evt.Info.PushName
			if title == "" {
				title = jid
			}
			body := "New message"
			if c.notifyShowPreview {
				switch {
				case mediaType != "" && text != "":
					body = fmt.Sprintf("[%s] %s", mediaType, text)
				case mediaType != "":
					body = fmt.Sprintf("[%s]", mediaType)
				case text != "":
					body = text
				}
			}
			if err := notify.Send(title, body); err != nil {
				c.log.Warnf("desktop notification failed: %v", err)
			}
		}
	}
}

// extractPreview trims plain conversational text for display. Kept for
// HistorySync ingestion, which only has a bare GetConversation()-shaped
// value at the point it's called (see classifyMessage for the fuller,
// type-aware version used everywhere else).
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

// classifyMessage identifies a message's type (empty string for plain
// text) and extracts its text content — the conversation/extended-text
// body for text messages, or the caption (if any) for media messages.
func classifyMessage(msg *waProto.Message) (mediaType, text string) {
	if msg == nil {
		return "", ""
	}
	switch {
	case msg.GetConversation() != "":
		return "", msg.GetConversation()
	case msg.GetExtendedTextMessage() != nil:
		return "", msg.GetExtendedTextMessage().GetText()
	case msg.GetImageMessage() != nil:
		return "image", msg.GetImageMessage().GetCaption()
	case msg.GetVideoMessage() != nil:
		return "video", msg.GetVideoMessage().GetCaption()
	case msg.GetAudioMessage() != nil:
		if msg.GetAudioMessage().GetPTT() {
			return "voice note", ""
		}
		return "audio", ""
	case msg.GetDocumentMessage() != nil:
		return "document", msg.GetDocumentMessage().GetCaption()
	case msg.GetStickerMessage() != nil:
		return "sticker", ""
	default:
		return "", ""
	}
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

// IsLoggedIn reports whether the underlying device store already has a
// persisted session, without opening a network connection.
func (c *Client) IsLoggedIn() bool {
	return c.WA.Store.ID != nil
}

// Contact is a simplified view of a WhatsApp contact.
type Contact struct {
	JID          string `json:"jid"`
	Name         string `json:"name,omitempty"` // best available: FullName > FirstName > PushName > BusinessName
	PushName     string `json:"pushName,omitempty"`
	BusinessName string `json:"businessName,omitempty"`
}

// ListContacts returns all locally known contacts, sorted by name. This
// reads directly from the local device store — no network connection
// needed, since WhatsApp syncs your contact list to the device during
// login and keeps it updated from there.
func (c *Client) ListContacts(ctx context.Context) ([]Contact, error) {
	contacts, err := c.WA.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, waerrors.Wrap(err, "loading contacts")
	}

	out := make([]Contact, 0, len(contacts))
	for jid, info := range contacts {
		out = append(out, Contact{
			JID:          jid.String(),
			Name:         bestContactName(info),
			PushName:     info.PushName,
			BusinessName: info.BusinessName,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func bestContactName(info types.ContactInfo) string {
	switch {
	case info.FullName != "":
		return info.FullName
	case info.FirstName != "":
		return info.FirstName
	case info.PushName != "":
		return info.PushName
	case info.BusinessName != "":
		return info.BusinessName
	default:
		return ""
	}
}

// Group is a simplified view of a WhatsApp group.
type Group struct {
	JID              string `json:"jid"`
	Name             string `json:"name"`
	Topic            string `json:"topic,omitempty"`
	ParticipantCount int    `json:"participantCount"`
}

// Participant is one member of a group.
type Participant struct {
	JID          string `json:"jid"`
	IsAdmin      bool   `json:"isAdmin,omitempty"`
	IsSuperAdmin bool   `json:"isSuperAdmin,omitempty"`
}

// ListGroups returns all groups the account is currently a member of.
func (c *Client) ListGroups(ctx context.Context) ([]Group, error) {
	groups, err := c.WA.GetJoinedGroups(ctx)
	if err != nil {
		return nil, waerrors.Wrap(err, "loading groups")
	}

	out := make([]Group, 0, len(groups))
	for _, g := range groups {
		out = append(out, Group{
			JID:              g.JID.String(),
			Name:             g.GroupName.Name,
			Topic:            g.Topic,
			ParticipantCount: len(g.Participants),
		})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// GroupInfo returns details and the participant list for one group.
func (c *Client) GroupInfo(ctx context.Context, jid types.JID) (Group, []Participant, error) {
	info, err := c.WA.GetGroupInfo(ctx, jid)
	if err != nil {
		return Group{}, nil, waerrors.Wrap(err, "loading group info")
	}

	group := Group{
		JID:              info.JID.String(),
		Name:             info.GroupName.Name,
		Topic:            info.Topic,
		ParticipantCount: len(info.Participants),
	}

	participants := make([]Participant, 0, len(info.Participants))
	for _, p := range info.Participants {
		participants = append(participants, Participant{
			JID:          p.JID.String(),
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
		})
	}
	return group, participants, nil
}

// CreateGroup creates a new group with the given name and participants.
func (c *Client) CreateGroup(ctx context.Context, name string, participants []types.JID) (Group, error) {
	info, err := c.WA.CreateGroup(ctx, whatsmeow.ReqCreateGroup{
		Name:         name,
		Participants: participants,
	})
	if err != nil {
		return Group{}, waerrors.Wrap(err, "creating group")
	}
	return Group{
		JID:              info.JID.String(),
		Name:             info.GroupName.Name,
		Topic:            info.Topic,
		ParticipantCount: len(info.Participants),
	}, nil
}

// AddParticipants adds participants to an existing group.
func (c *Client) AddParticipants(ctx context.Context, groupJID types.JID, participants []types.JID) error {
	_, err := c.WA.UpdateGroupParticipants(ctx, groupJID, participants, whatsmeow.ParticipantChangeAdd)
	if err != nil {
		return waerrors.Wrap(err, "adding participants")
	}
	return nil
}

// RemoveParticipants removes participants from a group.
func (c *Client) RemoveParticipants(ctx context.Context, groupJID types.JID, participants []types.JID) error {
	_, err := c.WA.UpdateGroupParticipants(ctx, groupJID, participants, whatsmeow.ParticipantChangeRemove)
	if err != nil {
		return waerrors.Wrap(err, "removing participants")
	}
	return nil
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
	if evt.Info.IsFromMe || isPseudoChat(evt.Info.Chat.String()) {
		return
	}

	sender := evt.Info.PushName
	if sender == "" {
		sender = evt.Info.Sender.User
	}

	mediaType, text := classifyMessage(evt.Message)
	switch {
	case mediaType != "" && text != "":
		text = fmt.Sprintf("[%s] %s", mediaType, text)
	case mediaType != "":
		text = fmt.Sprintf("[%s]", mediaType)
	case text == "":
		text = "[unsupported message type]"
	}

	ts := evt.Info.Timestamp.Local().Format("15:04:05")
	fmt.Printf("[%s] %s (%s): %s\n", ts, sender, evt.Info.Chat.String(), text)

	if guard != nil {
		if err := guard.MarkKnown(evt.Info.Sender.String()); err != nil {
			c.log.Warnf("failed to mark sender known: %v", err)
		}
	}
}

// Logout terminates the linked-device session on WhatsApp's servers and
// clears the local session, so a subsequent command needs `wa login`
// again. Returns waerrors.ErrNotLoggedIn if no session is active.
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

// Disconnect closes the current WhatsApp connection without invalidating
// the underlying session, so a later command can reconnect and resume
// using the same persisted device store.
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

// SendImage uploads data and sends it as an image message with an
// optional caption.
func (c *Client) SendImage(ctx context.Context, jid types.JID, data []byte, mimetype, caption string) (string, error) {
	upload, err := c.WA.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return "", waerrors.Wrap(err, "uploading image")
	}
	msg := &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			URL:           proto.String(upload.URL),
			DirectPath:    proto.String(upload.DirectPath),
			MediaKey:      upload.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: upload.FileEncSHA256,
			FileSHA256:    upload.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	}
	if caption != "" {
		msg.ImageMessage.Caption = proto.String(caption)
	}
	return c.sendMedia(ctx, jid, msg, "image", caption)
}

// SendVideo uploads data and sends it as a video message with an
// optional caption.
func (c *Client) SendVideo(ctx context.Context, jid types.JID, data []byte, mimetype, caption string) (string, error) {
	upload, err := c.WA.Upload(ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		return "", waerrors.Wrap(err, "uploading video")
	}
	msg := &waProto.Message{
		VideoMessage: &waProto.VideoMessage{
			URL:           proto.String(upload.URL),
			DirectPath:    proto.String(upload.DirectPath),
			MediaKey:      upload.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: upload.FileEncSHA256,
			FileSHA256:    upload.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	}
	if caption != "" {
		msg.VideoMessage.Caption = proto.String(caption)
	}
	return c.sendMedia(ctx, jid, msg, "video", caption)
}

// SendAudio uploads data and sends it as an audio message. Set voice=true
// to send as a voice note (PTT) rather than a regular audio file.
func (c *Client) SendAudio(ctx context.Context, jid types.JID, data []byte, mimetype string, voice bool) (string, error) {
	upload, err := c.WA.Upload(ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		return "", waerrors.Wrap(err, "uploading audio")
	}
	msg := &waProto.Message{
		AudioMessage: &waProto.AudioMessage{
			URL:           proto.String(upload.URL),
			DirectPath:    proto.String(upload.DirectPath),
			MediaKey:      upload.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: upload.FileEncSHA256,
			FileSHA256:    upload.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			PTT:           proto.Bool(voice),
		},
	}
	return c.sendMedia(ctx, jid, msg, "audio", "")
}

// SendDocument uploads data and sends it as a document message.
func (c *Client) SendDocument(ctx context.Context, jid types.JID, data []byte, mimetype, filename, caption string) (string, error) {
	upload, err := c.WA.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return "", waerrors.Wrap(err, "uploading document")
	}
	msg := &waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			URL:           proto.String(upload.URL),
			DirectPath:    proto.String(upload.DirectPath),
			MediaKey:      upload.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: upload.FileEncSHA256,
			FileSHA256:    upload.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			FileName:      proto.String(filename),
		},
	}
	if caption != "" {
		msg.DocumentMessage.Caption = proto.String(caption)
	}
	return c.sendMedia(ctx, jid, msg, "document", caption)
}

// SendSticker uploads data and sends it as a sticker message. data
// should already be in WebP format — WhatsApp doesn't accept other
// formats for stickers, and this doesn't convert for you.
func (c *Client) SendSticker(ctx context.Context, jid types.JID, data []byte, mimetype string) (string, error) {
	upload, err := c.WA.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return "", waerrors.Wrap(err, "uploading sticker")
	}
	msg := &waProto.Message{
		StickerMessage: &waProto.StickerMessage{
			URL:           proto.String(upload.URL),
			DirectPath:    proto.String(upload.DirectPath),
			MediaKey:      upload.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: upload.FileEncSHA256,
			FileSHA256:    upload.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	}
	return c.sendMedia(ctx, jid, msg, "sticker", "")
}

func (c *Client) sendMedia(ctx context.Context, jid types.JID, msg *waProto.Message, kind, caption string) (string, error) {
	var resp whatsmeow.SendResponse
	err := withRetry(func() error {
		var sendErr error
		resp, sendErr = c.WA.SendMessage(ctx, jid, msg)
		return sendErr
	})
	if err != nil {
		return "", waerrors.Wrap(err, "sending "+kind)
	}

	// Record the *actual* sent message (including its real upload
	// reference — URL/MediaKey/hashes) so it can be downloaded or
	// forwarded again later. This has to happen here rather than in the
	// cmd layer, which only has msgID and a caption string, not the
	// real uploaded message object.
	if c.msgs != nil {
		var raw string
		if b, err := proto.Marshal(msg); err == nil {
			raw = base64.StdEncoding.EncodeToString(b)
		}
		err := c.msgs.Append(msgstore.Message{
			ID:        resp.ID,
			ChatJID:   jid.String(),
			SenderJID: "me",
			Timestamp: time.Now().UnixMilli(),
			Text:      caption,
			FromMe:    true,
			MediaType: kind,
			RawProto:  raw,
		})
		if err != nil {
			c.log.Warnf("msgstore append failed for sent %s: %v", kind, err)
		}
	}

	return resp.ID, nil
}

// DownloadMedia reconstructs a stored message's media content from its
// RawProto and downloads it via WhatsApp's encrypted media CDN. Returns
// the raw bytes and the mimetype recorded with the message.
func (c *Client) DownloadMedia(ctx context.Context, quoted msgstore.Message) ([]byte, string, error) {
	original := decodeRawProto(quoted.RawProto)
	if original == nil {
		return nil, "", waerrors.New("original message content isn't available (too old, or not captured)")
	}

	var downloadable whatsmeow.DownloadableMessage
	var mimetype string
	switch {
	case original.GetImageMessage() != nil:
		downloadable = original.GetImageMessage()
		mimetype = original.GetImageMessage().GetMimetype()
	case original.GetVideoMessage() != nil:
		downloadable = original.GetVideoMessage()
		mimetype = original.GetVideoMessage().GetMimetype()
	case original.GetAudioMessage() != nil:
		downloadable = original.GetAudioMessage()
		mimetype = original.GetAudioMessage().GetMimetype()
	case original.GetDocumentMessage() != nil:
		downloadable = original.GetDocumentMessage()
		mimetype = original.GetDocumentMessage().GetMimetype()
	case original.GetStickerMessage() != nil:
		downloadable = original.GetStickerMessage()
		mimetype = original.GetStickerMessage().GetMimetype()
	default:
		return nil, "", waerrors.New("this message doesn't contain downloadable media")
	}

	data, err := c.WA.Download(ctx, downloadable)
	if err != nil {
		return nil, "", waerrors.Wrap(err, "downloading media")
	}
	return data, mimetype, nil
}

// ForwardMessage re-sends quoted's content to toJID, marked as forwarded.
// For media messages, this reuses the original upload (URL/MediaKey/
// hashes) rather than re-uploading — WhatsApp allows resending the same
// uploaded media reference with IsForwarded set.
func (c *Client) ForwardMessage(ctx context.Context, toJID types.JID, quoted msgstore.Message) (string, error) {
	original := decodeRawProto(quoted.RawProto)
	if original == nil {
		return "", waerrors.New("original message content isn't available to forward (too old, or not captured)")
	}

	msg, err := markForwarded(original)
	if err != nil {
		return "", err
	}

	var resp whatsmeow.SendResponse
	err = withRetry(func() error {
		var sendErr error
		resp, sendErr = c.WA.SendMessage(ctx, toJID, msg)
		return sendErr
	})
	if err != nil {
		return "", waerrors.Wrap(err, "forwarding message")
	}
	return resp.ID, nil
}

// markForwarded returns a message equivalent to original but with
// IsForwarded/ForwardingScore set on its ContextInfo. Plain Conversation
// text is normalized into ExtendedTextMessage first, since Conversation
// (a bare string field) has nowhere to attach ContextInfo. Supports text
// and all 5 media types; anything else is rejected.
func markForwarded(original *waProto.Message) (*waProto.Message, error) {
	bump := func(ci *waProto.ContextInfo) *waProto.ContextInfo {
		if ci == nil {
			ci = &waProto.ContextInfo{}
		}
		ci.IsForwarded = proto.Bool(true)
		ci.ForwardingScore = proto.Uint32(ci.GetForwardingScore() + 1)
		return ci
	}

	switch {
	case original.GetConversation() != "":
		return &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text:        proto.String(original.GetConversation()),
				ContextInfo: bump(nil),
			},
		}, nil
	case original.GetExtendedTextMessage() != nil:
		m := original.GetExtendedTextMessage()
		m.ContextInfo = bump(m.ContextInfo)
		return &waProto.Message{ExtendedTextMessage: m}, nil
	case original.GetImageMessage() != nil:
		m := original.GetImageMessage()
		m.ContextInfo = bump(m.ContextInfo)
		return &waProto.Message{ImageMessage: m}, nil
	case original.GetVideoMessage() != nil:
		m := original.GetVideoMessage()
		m.ContextInfo = bump(m.ContextInfo)
		return &waProto.Message{VideoMessage: m}, nil
	case original.GetAudioMessage() != nil:
		m := original.GetAudioMessage()
		m.ContextInfo = bump(m.ContextInfo)
		return &waProto.Message{AudioMessage: m}, nil
	case original.GetDocumentMessage() != nil:
		m := original.GetDocumentMessage()
		m.ContextInfo = bump(m.ContextInfo)
		return &waProto.Message{DocumentMessage: m}, nil
	case original.GetStickerMessage() != nil:
		m := original.GetStickerMessage()
		m.ContextInfo = bump(m.ContextInfo)
		return &waProto.Message{StickerMessage: m}, nil
	default:
		return nil, waerrors.New("forwarding this message type isn't supported yet")
	}
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
