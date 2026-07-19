package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/codebyoketch/wa-cli/internal/store"
	"github.com/codebyoketch/wa-cli/internal/whatsapp"
)

var groupCmd = &cobra.Command{
	Use:   "group",
	Short: "List, create, and manage WhatsApp groups",
}

var groupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List groups you're a member of",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := connectForGroups(ctx)
		if err != nil {
			return err
		}
		defer client.Disconnect()

		groups, err := client.ListGroups(ctx)
		if err != nil {
			return err
		}
		if len(groups) == 0 {
			fmt.Println("No groups found.")
			return nil
		}
		for _, g := range groups {
			fmt.Printf("%-30s %s (%d members)\n", g.Name, g.JID, g.ParticipantCount)
		}
		return nil
	},
}

var groupInfoCmd = &cobra.Command{
	Use:   "info <name-or-jid>",
	Short: "Show details and participants for one group",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := connectForGroups(ctx)
		if err != nil {
			return err
		}
		defer client.Disconnect()

		jid, err := resolveGroupJID(client, ctx, args[0])
		if err != nil {
			return err
		}

		group, participants, err := client.GroupInfo(ctx, jid)
		if err != nil {
			return err
		}

		fmt.Printf("Name:  %s\n", group.Name)
		fmt.Printf("JID:   %s\n", group.JID)
		if group.Topic != "" {
			fmt.Printf("Topic: %s\n", group.Topic)
		}
		fmt.Printf("Members (%d):\n", len(participants))
		for _, p := range participants {
			role := ""
			if p.IsSuperAdmin {
				role = " (owner)"
			} else if p.IsAdmin {
				role = " (admin)"
			}
			fmt.Printf("  %s%s\n", p.JID, role)
		}
		return nil
	},
}

var groupCreateCmd = &cobra.Command{
	Use:   "create <name> <participant...>",
	Short: "Create a new group",
	Long: `Create a new group. Each <participant> is a phone number, a saved
chat/contact name, or a JID. WhatsApp requires at least one participant
besides yourself.`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		name := args[0]
		targets := args[1:]

		jids, err := resolveJIDs(targets)
		if err != nil {
			return err
		}

		if !confirm(fmt.Sprintf("Create group %q with %d participant(s)?", name, len(jids))) {
			return fmt.Errorf("cancelled")
		}

		client, err := connectForGroups(ctx)
		if err != nil {
			return err
		}
		defer client.Disconnect()

		group, err := client.CreateGroup(ctx, name, jids)
		if err != nil {
			return err
		}
		fmt.Printf("Created %q (%s)\n", group.Name, group.JID)
		return nil
	},
}

var groupAddCmd = &cobra.Command{
	Use:   "add <group> <participant...>",
	Short: "Add participants to a group",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return groupParticipantChange(args[0], args[1:], true)
	},
}

var groupRemoveCmd = &cobra.Command{
	Use:   "remove <group> <participant...>",
	Short: "Remove participants from a group",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return groupParticipantChange(args[0], args[1:], false)
	},
}

func init() {
	groupCmd.AddCommand(groupListCmd, groupInfoCmd, groupCreateCmd, groupAddCmd, groupRemoveCmd)
	rootCmd.AddCommand(groupCmd)
}

func connectForGroups(ctx context.Context) (*whatsapp.Client, error) {
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
	if err := client.Connect(ctx); err != nil {
		return nil, err
	}
	return client, nil
}

// resolveGroupJID accepts a literal JID or a group name (exact match
// first, then substring), looked up against the account's joined groups.
func resolveGroupJID(client *whatsapp.Client, ctx context.Context, target string) (types.JID, error) {
	if strings.Contains(target, "@") {
		if jid, err := types.ParseJID(target); err == nil {
			return jid, nil
		}
	}

	groups, err := client.ListGroups(ctx)
	if err != nil {
		return types.JID{}, err
	}

	q := strings.ToLower(target)
	for _, g := range groups {
		if strings.EqualFold(g.Name, target) {
			return types.ParseJID(g.JID)
		}
	}
	for _, g := range groups {
		if strings.Contains(strings.ToLower(g.Name), q) {
			return types.ParseJID(g.JID)
		}
	}
	return types.JID{}, fmt.Errorf("no group found matching %q", target)
}

func resolveJIDs(targets []string) ([]types.JID, error) {
	jids := make([]types.JID, 0, len(targets))
	for _, t := range targets {
		jid, err := resolveJID(t)
		if err != nil {
			return nil, fmt.Errorf("resolving %q: %w", t, err)
		}
		jids = append(jids, jid)
	}
	return jids, nil
}

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	var resp string
	fmt.Scanln(&resp)
	return strings.ToLower(strings.TrimSpace(resp)) == "y"
}

func groupParticipantChange(groupTarget string, participantTargets []string, add bool) error {
	ctx := context.Background()
	client, err := connectForGroups(ctx)
	if err != nil {
		return err
	}
	defer client.Disconnect()

	groupJID, err := resolveGroupJID(client, ctx, groupTarget)
	if err != nil {
		return err
	}

	jids, err := resolveJIDs(participantTargets)
	if err != nil {
		return err
	}

	verb, prep := "Add", "to"
	if !add {
		verb, prep = "Remove", "from"
	}
	if !confirm(fmt.Sprintf("%s %d participant(s) %s group %s?", verb, len(jids), prep, groupJID.String())) {
		return fmt.Errorf("cancelled")
	}

	if add {
		err = client.AddParticipants(ctx, groupJID, jids)
	} else {
		err = client.RemoveParticipants(ctx, groupJID, jids)
	}
	if err != nil {
		return err
	}

	fmt.Println("Done.")
	return nil
}
