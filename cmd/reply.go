package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/codebyoketch/wa-cli/internal/msgstore"
)

var chatReplyCmd = &cobra.Command{
	Use:   "reply <recipient> <message-ref> <message...>",
	Short: "Reply to a specific message",
	Long: `Reply to a specific message in a chat. <message-ref> is the number
shown next to a message in 'wa chat open <recipient>' (e.g. "3"), or a raw
WhatsApp message ID if you already have one.`,
	Args: cobra.MinimumNArgs(3),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return completeChatNames(toComplete), cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		target := args[0]
		ref := args[1]
		text := strings.Join(args[2:], " ")

		jid, err := resolveJID(target)
		if err != nil {
			return err
		}

		ms := msgstore.New(a.Config.DataDir)
		quoted, err := resolveMessageRef(ms, jid.String(), ref)
		if err != nil {
			return err
		}

		if err := checkSendGuards(jid.String()); err != nil {
			return err
		}

		client, _, _, err := connectForSend(ctx)
		if err != nil {
			return err
		}
		defer client.Disconnect()

		msgID, err := client.SendReply(ctx, jid, text, quoted)
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
	chatCmd.AddCommand(chatReplyCmd)
}
