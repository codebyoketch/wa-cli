package cmd

import (
	"fmt"

	"github.com/codebyoketch/wa-cli/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View or change wa-cli configuration",
	Long:  "View or change wa-cli configuration stored in your user config directory.",
}

var configGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Print the current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.Path()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "config file: %s\n\n", path)
		fmt.Fprintf(out, "logLevel:   %s\n", a.Config.LogLevel)
		fmt.Fprintf(out, "jsonOutput: %t\n", a.Config.JSONOutput)
		fmt.Fprintf(out, "dataDir:    %s\n", a.Config.DataDir)
		fmt.Fprintf(out, "maxMessagesPerMinute: %d\n", a.Config.MaxMessagesPerMinute)
		fmt.Fprintf(out, "maxMessagesPerHour:   %d\n", a.Config.MaxMessagesPerHour)
		fmt.Fprintf(out, "maxMessagesPerDay:    %d\n", a.Config.MaxMessagesPerDay)
		fmt.Fprintf(out, "confirmNewRecipients: %t\n", a.Config.ConfirmNewRecipients)
		fmt.Fprintf(out, "notifyEnabled:        %t\n", a.Config.NotifyEnabled)
		fmt.Fprintf(out, "notifyGroups:         %t\n", a.Config.NotifyGroups)
		fmt.Fprintf(out, "notifyShowPreview:    %t\n", a.Config.NotifyShowPreview)
		return nil
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write out a default config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Save(config.Default()); err != nil {
			return err
		}
		path, err := config.Path()
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "wrote default config to %s\n", path)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configGetCmd, configInitCmd)
	rootCmd.AddCommand(configCmd)
}
