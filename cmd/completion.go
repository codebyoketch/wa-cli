package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate the autocompletion script for the specified shell",
	Long: `Generate the autocompletion script for wa-cli for the specified shell.
See each sub-command's help for details on how to use the generated script.

Completion is dynamic where it's cheap and safe to be: 'wa chat send',
'wa chat open/info/mute/unmute', 'wa chat reply', and 'wa chat forward'
complete chat names from your local chat cache, 'wa contact info'
completes from your local contact list, 'wa config get/set' completes
known config keys, and 'wa extension run/remove' completes installed
extension names — none of these open a WhatsApp connection, so they're
safe to trigger on every Tab press. Commands that need a live connection
to resolve their arguments ('wa group ...', 'wa media ...') fall back to
your shell's default (file) completion instead, since WhatsApp only
allows one active wa-cli connection at a time and a completion function
is the wrong place to compete for it (see ROADMAP.md's "Known issues").`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return cmd.Root().GenBashCompletionV2(os.Stdout, true)
		case "zsh":
			return cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			return cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

var completionBashCmd = &cobra.Command{
	Use:   "bash",
	Short: "Generate the autocompletion script for bash",
	Long: `Generate the autocompletion script for bash.

This script depends on the 'bash-completion' package. If it is not
already installed on your system, install it first.

To load completions in your current shell session:

	source <(wa completion bash)

To load completions for every new session, add that line to your
~/.bashrc (Linux) or ~/.bash_profile (macOS), or write it once to your
bash-completion directory:

	# Linux:
	wa completion bash > /etc/bash_completion.d/wa

	# macOS (Homebrew):
	wa completion bash > $(brew --prefix)/etc/bash_completion.d/wa

You will need to start a new shell for this setup to take effect.`,
	Args:                  cobra.NoArgs,
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Root().GenBashCompletionV2(os.Stdout, true)
	},
}

var completionZshCmd = &cobra.Command{
	Use:   "zsh",
	Short: "Generate the autocompletion script for zsh",
	Long: `Generate the autocompletion script for zsh.

If shell completion is not already enabled in your environment, enable
it first with:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions for every new session, write the script once to a
file in your $fpath, for example:

	wa completion zsh > "${fpath[1]}/_wa"

You will need to start a new shell for this setup to take effect.`,
	Args:                  cobra.NoArgs,
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Root().GenZshCompletion(os.Stdout)
	},
}

var completionFishCmd = &cobra.Command{
	Use:   "fish",
	Short: "Generate the autocompletion script for fish",
	Long: `Generate the autocompletion script for fish.

To load completions in your current shell session:

	wa completion fish | source

To load completions for every new session, write the script once to
your fish completions directory:

	wa completion fish > ~/.config/fish/completions/wa.fish

You will need to start a new shell for this setup to take effect.`,
	Args:                  cobra.NoArgs,
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Root().GenFishCompletion(os.Stdout, true)
	},
}

var completionPowerShellCmd = &cobra.Command{
	Use:   "powershell",
	Short: "Generate the autocompletion script for powershell",
	Long: `Generate the autocompletion script for powershell.

To load completions in your current shell session:

	wa completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add that line to your
PowerShell profile (see 'Get-Help about_Profiles' for its location).`,
	Args:                  cobra.NoArgs,
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
	},
}

func init() {
	completionCmd.AddCommand(completionBashCmd, completionZshCmd, completionFishCmd, completionPowerShellCmd)
	rootCmd.AddCommand(completionCmd)
}
