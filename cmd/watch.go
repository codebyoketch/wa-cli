package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/codebyoketch/wa-cli/internal/chatstore"
	"github.com/codebyoketch/wa-cli/internal/msgstore"
	"github.com/codebyoketch/wa-cli/internal/safety"
	"github.com/codebyoketch/wa-cli/internal/store"
	"github.com/codebyoketch/wa-cli/internal/whatsapp"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Stay connected and print new messages as they arrive",
	Long: `wa watch keeps a long-running connection to WhatsApp open and prints
new messages as they come in.

Unlike other commands (which connect briefly and disconnect), this runs in
the foreground until you stop it with Ctrl+C. It's also the most reliable
way to keep 'wa chat list' current if one-shot syncs aren't landing
consistently on your connection — it reconnects automatically on drops
rather than racing a short sync window each time.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		dbLog := waLog.Stdout("Database", "WARN", true)
		container, err := store.Open(ctx, a.Config.DataDir, dbLog)
		if err != nil {
			return err
		}

		cs := chatstore.New(a.Config.DataDir)
		guard := safety.New(a.Config.DataDir)

		client, err := whatsapp.New(ctx, container, dbLog, cs, msgstore.New(a.Config.DataDir))
		if err != nil {
			return err
		}
		if !client.IsLoggedIn() {
			return fmt.Errorf("not logged in: run 'wa login' first")
		}
		client.SetNotifications(a.Config.NotifyEnabled, a.Config.NotifyGroups, a.Config.NotifyShowPreview)

		return client.Watch(ctx, guard)
	},
}

func init() {
	rootCmd.AddCommand(watchCmd)
}
