// Package cmd implements wa-cli's Cobra command tree: one file per
// command or command group (chat.go, contact.go, group.go, media.go,
// config.go, extension.go, completion.go, login.go, logout.go,
// status.go, send.go/reply.go/forward.go, watch.go, version.go), each
// self-registering onto rootCmd from its own init(). root.go builds the
// shared *app.App once at package init time, before any command's RunE
// runs, so every command can assume a.Config/a.Log are ready.
//
// Everything here calls into internal/whatsapp for actual WhatsApp
// operations; this package's own logic is argument parsing, output
// formatting (see json.go), and the shared send-side checks in
// send_shared.go (recipient resolution, rate limiting, the
// confirm-new-recipient prompt). See ARCHITECTURE.md for the full
// package map.
package cmd

import (
	"fmt"
	"os"

	"github.com/codebyoketch/wa-cli/internal/app"
	"github.com/codebyoketch/wa-cli/internal/tui"
	"github.com/spf13/cobra"
)

var a *app.App

var rootCmd = &cobra.Command{
	Use:   "wa",
	Short: "wa is a WhatsApp client for your terminal",
	Long: `wa-cli lets you send and receive WhatsApp messages, manage chats,
contacts, and groups, all without leaving your terminal.

Running 'wa' with no subcommand opens a full-screen chat interface.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run(a)
	},
}

func init() {
	var err error
	a, err = app.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "wa: fatal:", err)
		os.Exit(1)
	}
}

// Execute runs the root command against os.Args. Called from main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
