package cmd

import (
	"fmt"

	"github.com/codebyoketch/wa-cli/internal/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the wa-cli version",
	Long:  "Print the wa-cli version, git commit, and build date.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), version.Get().String())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
