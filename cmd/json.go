package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

// jsonOutput is set by the --json flag. It's a plain package var rather
// than reading a.Config.JSONOutput directly at each call site so that a
// one-off `--json` on the command line can override the persistent
// config default (`wa config set jsonOutput true`) in either direction
// for a single invocation.
var jsonOutput bool

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false,
		"output as JSON instead of human-readable text (default: config's jsonOutput setting)")
}

// useJSON reports whether the current command should emit JSON. The
// --json flag always wins when set; otherwise it falls back to the
// jsonOutput config value, so someone who always wants JSON (e.g.
// scripting/agent use) can set it once with 'wa config set jsonOutput
// true' instead of passing --json on every call.
func useJSON(cmd *cobra.Command) bool {
	if cmd.Flags().Changed("json") {
		return jsonOutput
	}
	return a.Config.JSONOutput
}

// printJSON marshals v as indented JSON to cmd's stdout. Every JSON-mode
// branch across the CLI funnels through this one function so the
// indentation/encoding behavior can't drift between commands.
func printJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
