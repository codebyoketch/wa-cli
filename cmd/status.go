package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show login status",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Not logged in")
	},
}

// Execute executes the root command.
func init() {
	rootCmd.AddCommand(statusCmd)
}
