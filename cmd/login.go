package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/codebyoketch/wa-cli/internal/store"
	"github.com/codebyoketch/wa-cli/internal/whatsapp"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to WhatsApp",
	Long:  `Login to your WhatsApp account by scanning a QR code with your phone.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		dbLog := waLog.Stdout("Database", "WARN", true)

		container, err := store.Open(ctx, a.Config.DataDir, dbLog)
		if err != nil {
			return err
		}

		client, err := whatsapp.New(ctx, container, dbLog, nil)
		if err != nil {
			return err
		}

		if client.IsLoggedIn() {
			fmt.Println("Already logged in. Run 'wa logout' first to switch accounts.")
			return nil
		}

		if err := client.Login(ctx); err != nil {
			return err
		}

		client.Disconnect()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
