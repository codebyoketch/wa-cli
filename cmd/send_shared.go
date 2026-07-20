package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/codebyoketch/wa-cli/internal/chatstore"
	"github.com/codebyoketch/wa-cli/internal/msgstore"
	"github.com/codebyoketch/wa-cli/internal/ratelimit"
	"github.com/codebyoketch/wa-cli/internal/safety"
	"github.com/codebyoketch/wa-cli/internal/store"
	"github.com/codebyoketch/wa-cli/internal/whatsapp"
)

// completeChatNames returns chat names from the local chatstore cache
// matching the toComplete prefix, for use in ValidArgsFunction across
// every command that takes a chat name/JID as an argument. Reads the
// local JSON index only — never opens a WhatsApp connection — so it's
// safe to call on every Tab press without competing with 'wa watch' for
// WhatsApp's single-connection-per-device slot.
func completeChatNames(toComplete string) []string {
	cs := chatstore.New(a.Config.DataDir)
	chats, err := cs.List()
	if err != nil {
		return nil
	}

	q := strings.ToLower(toComplete)
	var names []string
	for _, c := range chats {
		if c.Name == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(c.Name), q) {
			names = append(names, c.Name)
		}
	}
	return names
}

// resolveJID accepts a literal WhatsApp JID (containing "@"), a chat
// name/partial name (resolved via chatstore, same lookup 'wa chat open'
// uses), or a bare phone number (with or without a leading "+"), which
// gets treated as an individual JID directly — useful when chatstore's
// local cache doesn't have this chat yet.
func resolveJID(target string) (types.JID, error) {
	if strings.Contains(target, "@") {
		if jid, err := types.ParseJID(target); err == nil {
			return jid, nil
		}
	}

	if chat, err := resolveChat(target); err == nil {
		return types.ParseJID(chat.JID)
	}

	digits := strings.TrimPrefix(target, "+")
	if digits != "" && isAllDigits(digits) {
		if jid, err := types.ParseJID(digits + "@s.whatsapp.net"); err == nil {
			return jid, nil
		}
	}

	return types.JID{}, fmt.Errorf(
		"no chat found matching %q, and it doesn't look like a phone number or JID — try 'wa chat list' first, or pass a full JID like 254700282181@s.whatsapp.net", target)
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// resolveMessageRef looks up a message by its position in 'wa chat open's
// numbered list (1-based) or, if ref doesn't parse as a number, treats it
// as a literal WhatsApp message ID.
func resolveMessageRef(ms *msgstore.Store, chatJID, ref string) (msgstore.Message, error) {
	msgs, err := ms.List(chatJID)
	if err != nil {
		return msgstore.Message{}, err
	}
	if idx, err := strconv.Atoi(ref); err == nil {
		if idx < 1 || idx > len(msgs) {
			return msgstore.Message{}, fmt.Errorf(
				"no message #%d in this chat's recent history — run 'wa chat open <chat>' to see numbered messages", idx)
		}
		return msgs[idx-1], nil
	}
	for _, m := range msgs {
		if m.ID == ref {
			return m, nil
		}
	}
	return msgstore.Message{}, fmt.Errorf("message %q not found in local history", ref)
}

// checkSendGuards enforces the rate limit and, if configured, prompts for
// confirmation before a first-time send to jidStr. Call this BEFORE
// connecting/sending.
func checkSendGuards(jidStr string) error {
	limiter := ratelimit.New(a.Config.DataDir, ratelimit.Config{
		PerMinute: a.Config.MaxMessagesPerMinute,
		PerHour:   a.Config.MaxMessagesPerHour,
		PerDay:    a.Config.MaxMessagesPerDay,
	})
	ok, retryAfter, window, err := limiter.Allow()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("rate limit hit (%s window) — try again in %s", window, retryAfter.Round(time.Second))
	}

	if a.Config.ConfirmNewRecipients {
		guard := safety.New(a.Config.DataDir)
		known, err := guard.IsKnown(jidStr)
		if err != nil {
			return err
		}
		if !known {
			fmt.Printf("You haven't messaged %s before. Send anyway? [y/N] ", jidStr)
			var resp string
			fmt.Scanln(&resp)
			if strings.ToLower(strings.TrimSpace(resp)) != "y" {
				return fmt.Errorf("cancelled")
			}
		}
	}
	return nil
}

// recordSendGuards records a successful send against the rate limiter and
// marks jidStr as a known recipient. Call this AFTER a send succeeds —
// never before, and never on failure.
func recordSendGuards(jidStr string) {
	limiter := ratelimit.New(a.Config.DataDir, ratelimit.Config{
		PerMinute: a.Config.MaxMessagesPerMinute,
		PerHour:   a.Config.MaxMessagesPerHour,
		PerDay:    a.Config.MaxMessagesPerDay,
	})
	if err := limiter.Record(); err != nil {
		a.Log.Warn("failed to record send for rate limiting", "error", err)
	}

	guard := safety.New(a.Config.DataDir)
	if err := guard.MarkKnown(jidStr); err != nil {
		a.Log.Warn("failed to mark recipient known", "error", err)
	}
}

// recordSentMessage saves our own outgoing message into msgstore so it
// can later be referenced by 'wa chat reply'/'wa chat forward' too.
// RawProto is set to a plain-text reconstruction of what was actually
// sent, so a message you sent via wa-cli can itself be forwarded later —
// without this, only received messages (captured via ingestMessage)
// would ever be forwardable.
func recordSentMessage(ms *msgstore.Store, jid types.JID, msgID, text string) {
	if ms == nil || msgID == "" {
		return
	}

	var raw string
	if b, err := proto.Marshal(&waProto.Message{Conversation: proto.String(text)}); err == nil {
		raw = base64.StdEncoding.EncodeToString(b)
	}

	err := ms.Append(msgstore.Message{
		ID:        msgID,
		ChatJID:   jid.String(),
		SenderJID: "me",
		Timestamp: time.Now().UnixMilli(),
		Text:      text,
		FromMe:    true,
		RawProto:  raw,
	})
	if err != nil {
		a.Log.Warn("failed to record sent message", "error", err)
	}
}

// connectForSend opens a session-store-backed, connected client ready to
// send. Callers must defer client.Disconnect().
func connectForSend(ctx context.Context) (*whatsapp.Client, *chatstore.Store, *msgstore.Store, error) {
	dbLog := waLog.Stdout("Database", "WARN", true)
	container, err := store.Open(ctx, a.Config.DataDir, dbLog)
	if err != nil {
		return nil, nil, nil, err
	}

	cs := chatstore.New(a.Config.DataDir)
	ms := msgstore.New(a.Config.DataDir)

	client, err := whatsapp.New(ctx, container, dbLog, cs, ms)
	if err != nil {
		return nil, nil, nil, err
	}
	if !client.IsLoggedIn() {
		return nil, nil, nil, fmt.Errorf("not logged in: run 'wa login' first")
	}
	if err := client.Connect(ctx); err != nil {
		return nil, nil, nil, err
	}

	return client, cs, ms, nil
}
