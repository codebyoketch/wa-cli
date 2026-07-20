package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/codebyoketch/wa-cli/internal/config"
	waerrors "github.com/codebyoketch/wa-cli/internal/errors"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View or change wa-cli configuration",
	Long:  "View or change wa-cli configuration stored in your user config directory.",
}

// configField is the single source of truth for every key `wa config
// get`/`set` knows about, so the two commands can't drift out of sync
// with each other or with config.Config.
type configField struct {
	name string
	get  func(c config.Config) string
	// set parses val and applies it to c, or returns an error naming
	// what was expected (e.g. "true/false", "a number").
	set func(c *config.Config, val string) error
}

var configFields = []configField{
	{
		name: "logLevel",
		get:  func(c config.Config) string { return c.LogLevel },
		set: func(c *config.Config, val string) error {
			switch val {
			case "debug", "info", "warn", "error":
				c.LogLevel = val
				return nil
			default:
				return fmt.Errorf("must be one of debug, info, warn, error (got %q)", val)
			}
		},
	},
	{
		name: "jsonOutput",
		get:  func(c config.Config) string { return strconv.FormatBool(c.JSONOutput) },
		set:  boolSetter(func(c *config.Config, v bool) { c.JSONOutput = v }),
	},
	{
		name: "dataDir",
		get:  func(c config.Config) string { return c.DataDir },
		set: func(c *config.Config, val string) error {
			if val == "" {
				return fmt.Errorf("must not be empty")
			}
			c.DataDir = val
			return nil
		},
	},
	{
		name: "maxMessagesPerMinute",
		get:  func(c config.Config) string { return strconv.Itoa(c.MaxMessagesPerMinute) },
		set:  intSetter(func(c *config.Config, v int) { c.MaxMessagesPerMinute = v }),
	},
	{
		name: "maxMessagesPerHour",
		get:  func(c config.Config) string { return strconv.Itoa(c.MaxMessagesPerHour) },
		set:  intSetter(func(c *config.Config, v int) { c.MaxMessagesPerHour = v }),
	},
	{
		name: "maxMessagesPerDay",
		get:  func(c config.Config) string { return strconv.Itoa(c.MaxMessagesPerDay) },
		set:  intSetter(func(c *config.Config, v int) { c.MaxMessagesPerDay = v }),
	},
	{
		name: "confirmNewRecipients",
		get:  func(c config.Config) string { return strconv.FormatBool(c.ConfirmNewRecipients) },
		set:  boolSetter(func(c *config.Config, v bool) { c.ConfirmNewRecipients = v }),
	},
	{
		name: "notifyEnabled",
		get:  func(c config.Config) string { return strconv.FormatBool(c.NotifyEnabled) },
		set:  boolSetter(func(c *config.Config, v bool) { c.NotifyEnabled = v }),
	},
	{
		name: "notifyGroups",
		get:  func(c config.Config) string { return strconv.FormatBool(c.NotifyGroups) },
		set:  boolSetter(func(c *config.Config, v bool) { c.NotifyGroups = v }),
	},
	{
		name: "notifyShowPreview",
		get:  func(c config.Config) string { return strconv.FormatBool(c.NotifyShowPreview) },
		set:  boolSetter(func(c *config.Config, v bool) { c.NotifyShowPreview = v }),
	},
}

// boolSetter and intSetter adapt a typed field setter to configField.set,
// so each field above only has to name its Go type once.
func boolSetter(apply func(*config.Config, bool)) func(*config.Config, string) error {
	return func(c *config.Config, val string) error {
		b, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("must be true or false (got %q)", val)
		}
		apply(c, b)
		return nil
	}
}

func intSetter(apply func(*config.Config, int)) func(*config.Config, string) error {
	return func(c *config.Config, val string) error {
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("must be a whole number (got %q)", val)
		}
		apply(c, n)
		return nil
	}
}

func findConfigField(name string) (configField, bool) {
	for _, f := range configFields {
		if strings.EqualFold(f.name, name) {
			return f, true
		}
	}
	return configField{}, false
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Print the current configuration, or a single key",
	Long: `Print the current configuration.

With no arguments, prints every key. With one argument, prints just that
key's value (handy for scripting, e.g. 'wa config get dataDir').`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		if len(args) == 1 {
			f, ok := findConfigField(args[0])
			if !ok {
				return fmt.Errorf("unknown config key %q (see 'wa config get' for the full list)", args[0])
			}
			fmt.Fprintln(out, f.get(a.Config))
			return nil
		}

		path, err := config.Path()
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "config file: %s\n\n", path)

		width := 0
		for _, f := range configFields {
			if len(f.name) > width {
				width = len(f.name)
			}
		}
		for _, f := range configFields {
			fmt.Fprintf(out, "%-*s  %s\n", width, f.name, f.get(a.Config))
		}
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Change a single configuration value",
	Long: `Change a single configuration value and save it immediately.

Run 'wa config get' with no arguments to see the full list of keys and
their current values.

Examples:
  wa config set notifyGroups false
  wa config set logLevel debug
  wa config set maxMessagesPerHour 200`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, val := args[0], args[1]

		f, ok := findConfigField(key)
		if !ok {
			names := make([]string, len(configFields))
			for i, cf := range configFields {
				names[i] = cf.name
			}
			sort.Strings(names)
			return fmt.Errorf("unknown config key %q\nvalid keys: %s", key, strings.Join(names, ", "))
		}

		if err := f.set(&a.Config, val); err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}
		if err := config.Save(a.Config); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%s = %s\n", f.name, f.get(a.Config))
		return nil
	},
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open the config file in $EDITOR",
	Long: `Open config.json in $EDITOR (falls back to $VISUAL, then "vi").

If no config file exists yet, one with default values is written first
so there's something to edit. After the editor exits, the file is
validated as JSON — if it's broken, wa-cli tells you and leaves the file
as you left it rather than silently discarding your edits.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.Path()
		if err != nil {
			return err
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := config.Save(config.Default()); err != nil {
				return waerrors.Wrap(err, "writing initial config")
			}
		} else if err != nil {
			return waerrors.Wrap(err, "checking config file")
		}

		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			editor = "vi"
		}

		editorCmd := exec.Command(editor, path)
		editorCmd.Stdin = os.Stdin
		editorCmd.Stdout = os.Stdout
		editorCmd.Stderr = os.Stderr
		if err := editorCmd.Run(); err != nil {
			return waerrors.Wrapf(err, "running editor %q", editor)
		}

		if _, err := config.Load(); err != nil {
			return fmt.Errorf("config file has invalid JSON after editing, fix it by hand or re-run 'wa config edit': %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "saved %s\n", path)
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
	configCmd.AddCommand(configGetCmd, configSetCmd, configEditCmd, configInitCmd)
	rootCmd.AddCommand(configCmd)
}
