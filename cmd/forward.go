package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/codebyoketch/wa-cli/internal/msgstore"
)

var chatForwardCmd = &cobra.Command{
	Use:   "forward <from-chat> <message-ref> <to-recipient>",
	Short: "Forward a message to another chat",
	Long: `Forward a message from one chat to another. <message-ref> is the
number shown in 'wa chat open <from-chat>', or a raw WhatsApp message ID.

Only plain text messages can be forwarded in this pass — media forwarding
isn't supported yet.`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fromTarget := args[0]
		ref := args[1]
		toTarget := args[2]

		fromJID, err := resolveJID(fromTarget)
		if err != nil {
			return err
		}
		toJID, err := resolveJID(toTarget)
		if err != nil {
			return err
		}

		ms := msgstore.New(a.Config.DataDir)
		quoted, err := resolveMessageRef(ms, fromJID.String(), ref)
		if err != nil {
			return err
		}

		if err := checkSendGuards(toJID.String()); err != nil {
			return err
		}

		client, _, _, err := connectForSend(ctx)
		if err != nil {
			return err
		}
		defer client.Disconnect()

		msgID, err := client.ForwardMessage(ctx, toJID, quoted)
		if err != nil {
			return err
		}

		recordSentMessage(ms, toJID, msgID, quoted.Text)
		recordSendGuards(toJID.String())

		fmt.Println("Forwarded.")
		return nil
	},
}

func init() {
	chatCmd.AddCommand(chatForwardCmd)
}
