package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/codebyoketch/wa-cli/internal/store"
	"github.com/codebyoketch/wa-cli/internal/whatsapp"
)

var contactCmd = &cobra.Command{
	Use:   "contact",
	Short: "List, search, and inspect your WhatsApp contacts",
	Long: `List, search, and inspect your WhatsApp contacts.

Unlike chat commands, these read your local contact list directly and
don't need a network connection — WhatsApp syncs contacts to the device
during login and keeps them updated from there.`,
}

var contactListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all contacts",
	RunE: func(cmd *cobra.Command, args []string) error {
		contacts, err := loadContacts()
		if err != nil {
			return err
		}
		printContacts(contacts)
		return nil
	},
}

var contactSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search contacts by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		contacts, err := loadContacts()
		if err != nil {
			return err
		}

		q := strings.ToLower(args[0])
		var results []whatsapp.Contact
		for _, c := range contacts {
			if strings.Contains(strings.ToLower(c.Name), q) ||
				strings.Contains(strings.ToLower(c.PushName), q) ||
				strings.Contains(c.JID, args[0]) {
				results = append(results, c)
			}
		}
		printContacts(results)
		return nil
	},
}

var contactInfoCmd = &cobra.Command{
	Use:   "info <name-or-jid>",
	Short: "Show details for one contact",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		contacts, err := loadContacts()
		if err != nil {
			return err
		}

		target := args[0]

		// Exact match first (JID or name).
		for _, c := range contacts {
			if c.JID == target || strings.EqualFold(c.Name, target) {
				printContactDetail(c)
				return nil
			}
		}

		// Fall back to substring match, take the first hit.
		q := strings.ToLower(target)
		for _, c := range contacts {
			if strings.Contains(strings.ToLower(c.Name), q) || strings.Contains(c.JID, target) {
				printContactDetail(c)
				return nil
			}
		}

		return fmt.Errorf("no contact found matching %q", target)
	},
}

func init() {
	contactCmd.AddCommand(contactListCmd, contactSearchCmd, contactInfoCmd)
	rootCmd.AddCommand(contactCmd)
}

func loadContacts() ([]whatsapp.Contact, error) {
	ctx := context.Background()
	dbLog := waLog.Stdout("Database", "WARN", true)

	container, err := store.Open(ctx, a.Config.DataDir, dbLog)
	if err != nil {
		return nil, err
	}

	client, err := whatsapp.New(ctx, container, dbLog, nil, nil)
	if err != nil {
		return nil, err
	}
	if !client.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in: run 'wa login' first")
	}

	return client.ListContacts(ctx)
}

func printContacts(contacts []whatsapp.Contact) {
	if len(contacts) == 0 {
		fmt.Println("No contacts found.")
		return
	}
	for _, c := range contacts {
		name := c.Name
		if name == "" {
			name = "(no name)"
		}
		fmt.Printf("%-30s %s\n", name, c.JID)
	}
}

func printContactDetail(c whatsapp.Contact) {
	fmt.Printf("JID:           %s\n", c.JID)
	if c.Name != "" {
		fmt.Printf("Name:          %s\n", c.Name)
	}
	if c.PushName != "" {
		fmt.Printf("Push name:     %s\n", c.PushName)
	}
	if c.BusinessName != "" {
		fmt.Printf("Business name: %s\n", c.BusinessName)
	}
}
