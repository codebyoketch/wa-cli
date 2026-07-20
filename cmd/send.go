package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var chatSendCmd = &cobra.Command{
	Use:   "send <recipient> <message...>",
	Short: "Send a text message",
	Long: `Send a plain text message. <recipient> can be a chat name (as shown
in 'wa chat list') or a literal WhatsApp JID.`,
	Args: cobra.MinimumNArgs(2),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return completeChatNames(toComplete), cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		target := args[0]
		text := strings.Join(args[1:], " ")

		jid, err := resolveJID(target)
		if err != nil {
			return err
		}

		if err := checkSendGuards(jid.String()); err != nil {
			return err
		}

		client, _, ms, err := connectForSend(ctx)
		if err != nil {
			return err
		}
		defer client.Disconnect()

		msgID, err := client.SendText(ctx, jid, text)
		if err != nil {
			return err
		}

		recordSentMessage(ms, jid, msgID, text)
		recordSendGuards(jid.String())

		fmt.Println("Sent.")
		return nil
	},
}

func init() {
	chatCmd.AddCommand(chatSendCmd)
}
