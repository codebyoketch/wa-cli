package cmd

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/codebyoketch/wa-cli/internal/msgstore"
)

var mediaCmd = &cobra.Command{
	Use:   "media",
	Short: "Send and download images, video, audio, documents, and stickers",
}

var mediaSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a media file",
}

// newMediaSendCmd builds one `wa media send <kind>` subcommand. kind must
// match one of internal/whatsapp's classifyMessage labels
// ("image"/"video"/"audio"/"document"/"sticker").
func newMediaSendCmd(kind string) *cobra.Command {
	var caption, filename string
	var voice bool

	cmd := &cobra.Command{
		Use:   kind + " <recipient> <file>",
		Short: "Send " + articleFor(kind) + " " + kind,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			target, path := args[0], args[1]

			data, mimetype, err := readMediaFile(path)
			if err != nil {
				return err
			}

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

			var msgID string
			switch kind {
			case "image":
				msgID, err = client.SendImage(ctx, jid, data, mimetype, caption)
			case "video":
				msgID, err = client.SendVideo(ctx, jid, data, mimetype, caption)
			case "audio":
				msgID, err = client.SendAudio(ctx, jid, data, mimetype, voice)
			case "document":
				name := filename
				if name == "" {
					name = filepath.Base(path)
				}
				msgID, err = client.SendDocument(ctx, jid, data, mimetype, name, caption)
			case "sticker":
				msgID, err = client.SendSticker(ctx, jid, data, mimetype)
			}
			if err != nil {
				return err
			}

			label := "[" + kind + "]"
			if caption != "" {
				label += " " + caption
			}
			recordSentMessage(ms, jid, msgID, label)
			recordSendGuards(jid.String())

			fmt.Println("Sent.")
			return nil
		},
	}

	switch kind {
	case "image", "video", "document":
		cmd.Flags().StringVar(&caption, "caption", "", "optional caption")
	}
	if kind == "document" {
		cmd.Flags().StringVar(&filename, "filename", "", "filename shown to the recipient (defaults to the local file's name)")
	}
	if kind == "audio" {
		cmd.Flags().BoolVar(&voice, "voice", false, "send as a voice note (PTT) instead of a regular audio file")
	}

	return cmd
}

func articleFor(kind string) string {
	if kind == "audio" || kind == "image" {
		return "an"
	}
	return "a"
}

var mediaDownloadCmd = &cobra.Command{
	Use:   "download <chat> <message-ref> [output-path]",
	Short: "Download media from a message",
	Long: `Download media from a message. <message-ref> is the number shown in
'wa chat open <chat>', or a raw WhatsApp message ID. If output-path is
omitted, a filename is derived from the message ID and the media's type.`,
	Args: cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		target, ref := args[0], args[1]

		jid, err := resolveJID(target)
		if err != nil {
			return err
		}

		ms := msgstore.New(a.Config.DataDir)
		msg, err := resolveMessageRef(ms, jid.String(), ref)
		if err != nil {
			return err
		}

		client, err := connectClient(ctx)
		if err != nil {
			return err
		}
		defer client.Disconnect()

		data, mimetype, err := client.DownloadMedia(ctx, msg)
		if err != nil {
			return err
		}

		outPath := ""
		if len(args) == 3 {
			outPath = args[2]
		} else {
			outPath = defaultDownloadPath(msg, mimetype)
		}

		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return fmt.Errorf("writing file: %w", err)
		}

		fmt.Printf("Saved to %s (%d bytes)\n", outPath, len(data))
		return nil
	},
}

var mediaListCmd = &cobra.Command{
	Use:   "list <chat>",
	Short: "List media messages in a chat's local history",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		chat, err := resolveChat(args[0])
		if err != nil {
			return err
		}

		ms := msgstore.New(a.Config.DataDir)
		msgs, err := ms.List(chat.JID)
		if err != nil {
			return err
		}

		found := false
		for i, m := range msgs {
			if m.MediaType == "" {
				continue
			}
			found = true
			who := "them"
			if m.FromMe {
				who = "you"
			}
			ts := time.UnixMilli(m.Timestamp).Local().Format("15:04:05")
			fmt.Printf("[%d] (%s) %s sent %s %s", i+1, ts, who, articleFor(m.MediaType), m.MediaType)
			if m.Text != "" {
				fmt.Printf(": %s", m.Text)
			}
			fmt.Println()
		}
		if !found {
			fmt.Println("No media messages in local history for this chat.")
		}
		return nil
	},
}

func init() {
	mediaSendCmd.AddCommand(
		newMediaSendCmd("image"),
		newMediaSendCmd("video"),
		newMediaSendCmd("audio"),
		newMediaSendCmd("document"),
		newMediaSendCmd("sticker"),
	)
	mediaCmd.AddCommand(mediaSendCmd, mediaDownloadCmd, mediaListCmd)
	rootCmd.AddCommand(mediaCmd)
}

// readMediaFile reads path and guesses its mimetype from the file
// extension, falling back to content sniffing.
func readMediaFile(path string) ([]byte, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("reading file: %w", err)
	}
	mimetype := mime.TypeByExtension(filepath.Ext(path))
	if mimetype == "" {
		mimetype = http.DetectContentType(data)
	}
	return data, mimetype, nil
}

// defaultDownloadPath derives a filename from the message ID and a
// guessed extension for mimetype, used when no output path is given.
func defaultDownloadPath(msg msgstore.Message, mimetype string) string {
	name := msg.ID
	if name == "" {
		name = fmt.Sprintf("%d", msg.Timestamp)
	}
	if exts, err := mime.ExtensionsByType(mimetype); err == nil && len(exts) > 0 {
		return name + exts[0]
	}
	return name
}
