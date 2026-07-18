package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to WhatsApp",
	Long:  `Login to your WhatsApp account.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Logging in to WhatsApp...")
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
