package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/codebyoketch/wa-cli/internal/store"
	"github.com/codebyoketch/wa-cli/internal/whatsapp"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout of WhatsApp",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		dbLog := waLog.Stdout("Database", "WARN", true)

		container, err := store.Open(ctx, a.Config.DataDir, dbLog)
		if err != nil {
			return err
		}

		client, err := whatsapp.New(ctx, container, dbLog, nil, nil)
		if err != nil {
			return err
		}

		if !client.IsLoggedIn() {
			fmt.Println("Not logged in.")
			return nil
		}

		if err := client.Connect(ctx); err != nil {
			return err
		}
		defer client.Disconnect()

		if err := client.Logout(ctx); err != nil {
			return err
		}

		fmt.Println("Logged out.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}
