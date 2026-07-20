package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/codebyoketch/wa-cli/internal/extension"
)

var extensionCmd = &cobra.Command{
	Use:     "extension",
	Aliases: []string{"ext"},
	Short:   "Manage wa-cli extensions",
	Long: `Manage wa-cli extensions.

An extension is a git repository containing a wa-extension.json manifest
(name, description, entrypoint) plus a single executable entrypoint.
Installed extensions run as ordinary subprocesses via 'wa extension run'.`,
}

// completeExtensionNames returns installed extension names matching the
// toComplete prefix, for use in ValidArgsFunction on 'wa extension
// run'/'remove'. extension.List() only reads the local extensions
// directory — no network access — so it's safe on every Tab press.
func completeExtensionNames(toComplete string) []string {
	exts, _ := extension.List()
	q := strings.ToLower(toComplete)
	var names []string
	for _, e := range exts {
		if strings.HasPrefix(strings.ToLower(e.Name), q) {
			names = append(names, e.Name)
		}
	}
	return names
}

var extensionInstallCmd = &cobra.Command{
	Use:   "install <source>",
	Short: "Install an extension from a git repository",
	Long: `Install an extension from a git repository.

<source> is passed straight to 'git clone', so anything git accepts
works: an https:// URL, a git@ URL, or a local path. The repo must
contain a wa-extension.json manifest at its root naming the extension
and its entrypoint executable.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ext, err := extension.Install(args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Installed %s.\n", ext.Name)
		if ext.Description != "" {
			fmt.Fprintln(cmd.OutOrStdout(), ext.Description)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Run it with: wa extension run %s\n", ext.Name)
		return nil
	},
}

var extensionListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List installed extensions",
	RunE: func(cmd *cobra.Command, args []string) error {
		exts, errs := extension.List()
		out := cmd.OutOrStdout()

		for _, err := range errs {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", err)
		}

		sort.Slice(exts, func(i, j int) bool { return exts[i].Name < exts[j].Name })

		if useJSON(cmd) {
			if exts == nil {
				exts = []extension.Extension{}
			}
			return printJSON(cmd, exts)
		}

		if len(exts) == 0 {
			fmt.Fprintln(out, "No extensions installed. Install one with 'wa extension install <git-url>'.")
			return nil
		}

		width := 0
		for _, e := range exts {
			if len(e.Name) > width {
				width = len(e.Name)
			}
		}
		for _, e := range exts {
			desc := e.Description
			if e.Version != "" {
				desc = fmt.Sprintf("%s (%s)", desc, e.Version)
			}
			fmt.Fprintf(out, "%-*s  %s\n", width, e.Name, desc)
		}
		return nil
	},
}

var extensionRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm", "uninstall"},
	Short:   "Remove an installed extension",
	Args:    cobra.ExactArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return completeExtensionNames(toComplete), cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := extension.Remove(args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed %s.\n", args[0])
		return nil
	},
}

var extensionRunCmd = &cobra.Command{
	Use:   "run <name> [-- args...]",
	Short: "Run an installed extension",
	Long: `Run an installed extension, passing any remaining arguments straight
through to it.

Example:
  wa extension run wa-hello -- --loud`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true, // everything after <name> belongs to the extension, not wa-cli
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return completeExtensionNames(toComplete), cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		extArgs := args[1:]
		if len(extArgs) > 0 && extArgs[0] == "--" {
			extArgs = extArgs[1:]
		}
		return extension.Run(name, extArgs)
	},
}

func init() {
	extensionCmd.AddCommand(extensionInstallCmd, extensionListCmd, extensionRemoveCmd, extensionRunCmd)
	rootCmd.AddCommand(extensionCmd)
}
