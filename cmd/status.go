package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/codebyoketch/wa-cli/internal/store"
	"github.com/codebyoketch/wa-cli/internal/whatsapp"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show login status",
	RunE: func(cmd *cobra.Command, args []string) error {
		var loggedIn bool
		var user string
		err := captureLibraryStdout(func() error {
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

			loggedIn = client.IsLoggedIn()
			if loggedIn {
				user = client.WA.Store.ID.User
			}
			return nil
		})
		if err != nil {
			return err
		}

		if useJSON(cmd) {
			return printJSON(cmd, struct {
				LoggedIn bool   `json:"loggedIn"`
				User     string `json:"user,omitempty"`
			}{loggedIn, user})
		}

		if !loggedIn {
			fmt.Println("Not logged in. Run 'wa login' to get started.")
			return nil
		}
		fmt.Printf("Logged in as %s\n", user)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
