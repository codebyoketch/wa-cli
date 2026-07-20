package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/codebyoketch/wa-cli/internal/chatstore"
	"github.com/codebyoketch/wa-cli/internal/msgstore"
	"github.com/codebyoketch/wa-cli/internal/store"
	"github.com/codebyoketch/wa-cli/internal/whatsapp"
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "List, open, search, and inspect chats",
}

var chatListNoSync bool

var chatListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your chats, most recent first",
	RunE: func(cmd *cobra.Command, args []string) error {
		if chatListNoSync {
			cs := chatstore.New(a.Config.DataDir)
			chats, err := cs.List()
			if err != nil {
				return err
			}
			return outputChats(cmd, chats)
		}

		chats, err := syncAndLoadChats(cmd)
		if err != nil {
			return err
		}
		return outputChats(cmd, chats)
	},
}

func init() {
	chatListCmd.Flags().BoolVar(&chatListNoSync, "no-sync", false,
		"read the local chat cache without connecting — use this if 'wa watch' is already running, since WhatsApp only allows one active connection per device")
}

var chatSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search chats by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cs := chatstore.New(a.Config.DataDir)
		results, err := cs.Search(args[0])
		if err != nil {
			return err
		}
		return outputChats(cmd, results)
	},
}

var chatInfoCmd = &cobra.Command{
	Use:   "info <jid-or-name>",
	Short: "Show details for one chat",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		chat, err := resolveChat(args[0])
		if err != nil {
			return err
		}
		if useJSON(cmd) {
			return printJSON(cmd, chat)
		}
		fmt.Printf("JID:            %s\n", chat.JID)
		fmt.Printf("Name:           %s\n", chat.Name)
		fmt.Printf("Group:          %t\n", chat.IsGroup)
		fmt.Printf("Unread:         %d\n", chat.UnreadCount)
		if chat.LastMessageAt > 0 {
			fmt.Printf("Last message:   %s\n", time.UnixMilli(chat.LastMessageAt).Format(time.RFC1123))
		}
		if chat.LastMessagePreview != "" {
			fmt.Printf("Last preview:   %s\n", chat.LastMessagePreview)
		}
		return nil
	},
}

var chatOpenCmd = &cobra.Command{
	Use:   "open <jid-or-name>",
	Short: "Open a chat and show recent message history",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		chat, err := resolveChat(args[0])
		if err != nil {
			return err
		}
		cs := chatstore.New(a.Config.DataDir)
		if err := cs.MarkRead(chat.JID); err != nil {
			a.Log.Warn("failed to mark chat read", "error", err)
		}
		fmt.Printf("%s (%s)\n\n", chat.Name, chat.JID)

		ms := msgstore.New(a.Config.DataDir)
		msgs, err := ms.List(chat.JID)
		if err != nil {
			return err
		}
		if useJSON(cmd) {
			return printJSON(cmd, struct {
				Chat     chatstore.Chat     `json:"chat"`
				Messages []msgstore.Message `json:"messages"`
			}{chat, msgs})
		}
		if len(msgs) == 0 {
			fmt.Println("No local message history yet. Run 'wa watch' for a while, or send/receive a message, to build it up.")
			return nil
		}
		for i, m := range msgs {
			who := "them"
			if m.FromMe {
				who = "you"
			}
			text := m.Text
			switch {
			case m.MediaType != "" && text != "":
				text = fmt.Sprintf("[%s] %s", m.MediaType, text)
			case m.MediaType != "":
				text = fmt.Sprintf("[%s]", m.MediaType)
			}
			ts := time.UnixMilli(m.Timestamp).Local().Format("15:04:05")
			fmt.Printf("[%d] (%s) %s: %s\n", i+1, ts, who, text)
		}
		fmt.Println("\nUse the [n] number with 'wa chat reply' or 'wa chat forward' to reference a message.")
		return nil
	},
}

var chatMuteCmd = &cobra.Command{
	Use:   "mute <chat>",
	Short: "Suppress desktop notifications for one chat",
	Long: `Suppress desktop notifications for one chat, in wa watch and the TUI.
Unrelated to unread counts or wa chat list — a muted chat still shows up
normally, it just won't pop a desktop notification.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		chat, err := resolveChat(args[0])
		if err != nil {
			return err
		}
		cs := chatstore.New(a.Config.DataDir)
		if err := cs.SetMuted(chat.JID, true); err != nil {
			return err
		}
		fmt.Printf("Muted %s.\n", chat.JID)
		return nil
	},
}

var chatUnmuteCmd = &cobra.Command{
	Use:   "unmute <chat>",
	Short: "Re-enable desktop notifications for one chat",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		chat, err := resolveChat(args[0])
		if err != nil {
			return err
		}
		cs := chatstore.New(a.Config.DataDir)
		if err := cs.SetMuted(chat.JID, false); err != nil {
			return err
		}
		fmt.Printf("Unmuted %s.\n", chat.JID)
		return nil
	},
}

func init() {
	chatCmd.AddCommand(chatListCmd, chatSearchCmd, chatInfoCmd, chatOpenCmd, chatMuteCmd, chatUnmuteCmd)
	rootCmd.AddCommand(chatCmd)
}

// syncAndLoadChats connects briefly to pick up any new history/messages,
// then reads back the local chat store. Kept short (5s) since this runs
// on every `wa chat list` call — `wa watch` (Phase 5) is the long-running
// alternative for staying continuously in sync.
func syncAndLoadChats(cmd *cobra.Command) ([]chatstore.Chat, error) {
	var chats []chatstore.Chat
	err := captureLibraryStdout(func() error {
		ctx := context.Background()
		dbLog := waLog.Stdout("Database", "WARN", true)

		container, err := store.Open(ctx, a.Config.DataDir, dbLog)
		if err != nil {
			return err
		}

		cs := chatstore.New(a.Config.DataDir)
		client, err := whatsapp.New(ctx, container, dbLog, cs, msgstore.New(a.Config.DataDir))
		if err != nil {
			return err
		}

		if err := client.SyncChats(ctx, 2*time.Second); err != nil {
			return err
		}

		chats, err = cs.List()
		return err
	})
	return chats, err
}

// resolveChat looks up a chat by exact JID first, falling back to a name
// search, then a bare phone-number JID (same fallback resolveJID uses for
// send/reply/forward) — so open/info work on an uncached number too, not
// just chats already synced into chatstore. A phone-number fallback
// returns a bare Chat with no cached name/history, not an error.
func resolveChat(target string) (chatstore.Chat, error) {
	cs := chatstore.New(a.Config.DataDir)

	if chat, ok, err := cs.Get(target); err != nil {
		return chatstore.Chat{}, err
	} else if ok {
		return chat, nil
	}

	results, err := cs.Search(target)
	if err != nil {
		return chatstore.Chat{}, err
	}
	if len(results) > 0 {
		return results[0], nil
	}

	digits := strings.TrimPrefix(target, "+")
	if digits != "" && isAllDigits(digits) {
		return chatstore.Chat{JID: digits + "@s.whatsapp.net"}, nil
	}

	return chatstore.Chat{}, fmt.Errorf("no chat found matching %q — try 'wa chat list' first to sync", target)
}

// outputChats writes chats as JSON if requested, otherwise falls back to
// the existing human-readable printChats.
func outputChats(cmd *cobra.Command, chats []chatstore.Chat) error {
	if useJSON(cmd) {
		if chats == nil {
			chats = []chatstore.Chat{} // marshal as [] not null
		}
		return printJSON(cmd, chats)
	}
	printChats(chats)
	return nil
}

func printChats(chats []chatstore.Chat) {
	if len(chats) == 0 {
		fmt.Println("No chats found. Try again after a moment — history may still be syncing.")
		return
	}
	for _, c := range chats {
		unread := ""
		if c.UnreadCount > 0 {
			unread = fmt.Sprintf(" (%d unread)", c.UnreadCount)
		}
		fmt.Printf("%-30s %s%s\n", c.Name, c.JID, unread)
		if c.LastMessagePreview != "" {
			fmt.Printf("  ↳ %s\n", c.LastMessagePreview)
		}
	}
}
