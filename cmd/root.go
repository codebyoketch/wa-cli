package cmd

import (
	"fmt"
	"os"

	"github.com/codebyoketch/wa-cli/internal/app"
	"github.com/spf13/cobra"
)

var a *app.App

var rootCmd = &cobra.Command{
	Use:   "wa",
	Short: "wa is a WhatsApp client for your terminal",
	Long: `wa-cli lets you send and receive WhatsApp messages, manage chats,
contacts, and groups, all without leaving your terminal.`,
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
