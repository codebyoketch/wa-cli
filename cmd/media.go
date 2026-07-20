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

			if kind == "audio" && voice {
				if !looksLikeOgg(data) {
					return fmt.Errorf(
						"--voice requires an OGG/Opus file — WhatsApp voice notes need the exact mimetype "+
							`"audio/ogg; codecs=opus", and %s doesn't look like an Ogg container. `+
							"Convert it first, e.g.: ffmpeg -i %s -c:a libopus -b:a 32k voice.ogg", path, path)
				}
				// WhatsApp requires this exact string for a PTT bubble to
				// render/play correctly. Neither extension-based mimetype
				// lookup (bare "audio/ogg") nor content-sniffing
				// ("application/ogg") produce it, so it's forced here
				// rather than trusted from readMediaFile.
				mimetype = "audio/ogg; codecs=opus"
			}
			if kind == "sticker" && mimetype != "image/webp" {
				return fmt.Errorf(
					"stickers must be WebP images (got %q for %s) — WhatsApp rejects other formats for stickers; "+
						"convert first, e.g.: ffmpeg -i %s -vf scale=512:512 sticker.webp", mimetype, path, path)
			}

			jid, err := resolveJID(target)
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

			switch kind {
			case "image":
				_, err = client.SendImage(ctx, jid, data, mimetype, caption)
			case "video":
				_, err = client.SendVideo(ctx, jid, data, mimetype, caption)
			case "audio":
				_, err = client.SendAudio(ctx, jid, data, mimetype, voice)
			case "document":
				name := filename
				if name == "" {
					name = filepath.Base(path)
				}
				_, err = client.SendDocument(ctx, jid, data, mimetype, name, caption)
			case "sticker":
				_, err = client.SendSticker(ctx, jid, data, mimetype)
			}
			if err != nil {
				return err
			}

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

		var mediaMsgs []msgstore.Message
		for _, m := range msgs {
			if m.MediaType != "" {
				mediaMsgs = append(mediaMsgs, m)
			}
		}

		if useJSON(cmd) {
			if mediaMsgs == nil {
				mediaMsgs = []msgstore.Message{}
			}
			return printJSON(cmd, mediaMsgs)
		}

		if len(mediaMsgs) == 0 {
			fmt.Println("No media messages in local history for this chat.")
			return nil
		}
		for _, m := range mediaMsgs {
			// i+1 in the old output referenced the position within the
			// full message history (matching 'wa chat open's numbering,
			// which 'wa media download' accepts as <message-ref>), not
			// the position within this filtered list — preserve that by
			// looking the message back up in msgs.
			idx := 0
			for j, full := range msgs {
				if full.ID == m.ID {
					idx = j + 1
					break
				}
			}
			who := "them"
			if m.FromMe {
				who = "you"
			}
			ts := time.UnixMilli(m.Timestamp).Local().Format("15:04:05")
			fmt.Printf("[%d] (%s) %s sent %s %s", idx, ts, who, articleFor(m.MediaType), m.MediaType)
			if m.Text != "" {
				fmt.Printf(": %s", m.Text)
			}
			fmt.Println()
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

// looksLikeOgg reports whether data starts with the Ogg container magic
// bytes ("OggS"). This doesn't confirm the stream is Opus-encoded
// specifically (Ogg can carry Vorbis, FLAC, etc.), but it catches the
// common mistake of pointing --voice at an mp3/wav/m4a file, which
// WhatsApp will accept upload-wise but then fail to play as a voice note.
func looksLikeOgg(data []byte) bool {
	return len(data) >= 4 && string(data[:4]) == "OggS"
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
