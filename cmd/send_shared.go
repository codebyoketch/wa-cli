package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/codebyoketch/wa-cli/internal/chatstore"
	"github.com/codebyoketch/wa-cli/internal/msgstore"
	"github.com/codebyoketch/wa-cli/internal/ratelimit"
	"github.com/codebyoketch/wa-cli/internal/safety"
	"github.com/codebyoketch/wa-cli/internal/store"
	"github.com/codebyoketch/wa-cli/internal/whatsapp"
)

// resolveJID accepts either a literal WhatsApp JID (containing "@") or a
// chat name/partial name, resolving the latter via chatstore — the same
// lookup 'wa chat open' uses.
func resolveJID(target string) (types.JID, error) {
	if strings.Contains(target, "@") {
		if jid, err := types.ParseJID(target); err == nil {
			return jid, nil
		}
	}
	chat, err := resolveChat(target)
	if err != nil {
		return types.JID{}, err
	}
	return types.ParseJID(chat.JID)
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
func recordSentMessage(ms *msgstore.Store, jid types.JID, msgID, text string) {
	if ms == nil || msgID == "" {
		return
	}
	err := ms.Append(msgstore.Message{
		ID:        msgID,
		ChatJID:   jid.String(),
		SenderJID: "me",
		Timestamp: time.Now().UnixMilli(),
		Text:      text,
		FromMe:    true,
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
