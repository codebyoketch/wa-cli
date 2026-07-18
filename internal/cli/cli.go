// Package cli is a small, dependency-free command router that mimics the
// parts of Cobra's UX wa-cli needs (Use/Short/Long, --help, subcommands).
//
// It exists purely because the sandbox this scaffold was generated in
// couldn't reach proxy.golang.org / gopkg.in to fetch Cobra's dependency
// graph. Swapping to real Cobra later is a mechanical change:
//
//	go get github.com/spf13/cobra@latest
//
// then replace *cli.Command construction in internal/app with
// *cobra.Command construction — the Run signatures below are already
// Cobra-compatible: func(cmd *Command, args []string) error.
package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Command is one CLI command or subcommand.
type Command struct {
	// Use is the one-line usage string, e.g. "send <chat> <message>".
	Use string
	// Short is a one-line description shown in parent help listings.
	Short string
	// Long is a longer description shown in this command's own --help.
	Long string
	// Run is executed if this command has no matching subcommand in args.
	Run func(cmd *Command, args []string) error

	commands []*Command
	parent   *Command
	out      io.Writer
}

// Root creates a new top-level command, writing help/usage to stdout.
func Root(use, short, long string) *Command {
	return &Command{Use: use, Short: short, Long: long, out: os.Stdout}
}

// AddCommand registers one or more subcommands.
func (c *Command) AddCommand(subs ...*Command) {
	for _, s := range subs {
		s.parent = c
		c.commands = append(c.commands, s)
	}
}

// Name returns the command's name (the first word of Use).
func (c *Command) Name() string {
	if i := strings.IndexByte(c.Use, ' '); i >= 0 {
		return c.Use[:i]
	}
	return c.Use
}

// Out returns the writer help/usage text goes to (stdout by default).
func (c *Command) Out() io.Writer {
	if c.out != nil {
		return c.out
	}
	if c.parent != nil {
		return c.parent.Out()
	}
	return os.Stdout
}

// commandPath returns e.g. "wa config set" for nested commands.
func (c *Command) commandPath() string {
	if c.parent == nil {
		return c.Name()
	}
	return c.parent.commandPath() + " " + c.Name()
}

// Execute runs the root command against os.Args[1:].
func (c *Command) Execute() error {
	return c.execute(os.Args[1:])
}

func (c *Command) execute(args []string) error {
	// Find a matching subcommand as the first non-flag argument.
	if len(args) > 0 {
		first := args[0]
		if first == "-h" || first == "--help" {
			c.printHelp()
			return nil
		}
		for _, sub := range c.commands {
			if sub.Name() == first {
				return sub.execute(args[1:])
			}
		}
	}

	if len(args) == 0 && c.Run == nil && len(c.commands) > 0 {
		c.printHelp()
		return nil
	}

	if c.Run == nil {
		if len(args) > 0 {
			fmt.Fprintf(c.Out(), "unknown command %q for %q\n\n", args[0], c.commandPath())
			c.printHelp()
			return fmt.Errorf("unknown command: %s", args[0])
		}
		c.printHelp()
		return nil
	}

	return c.Run(c, args)
}

func (c *Command) printHelp() {
	w := c.Out()
	if c.Long != "" {
		fmt.Fprintln(w, c.Long)
	} else if c.Short != "" {
		fmt.Fprintln(w, c.Short)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Usage:\n  %s", c.commandPath())
	if len(c.commands) > 0 {
		fmt.Fprint(w, " [command]")
	}
	fmt.Fprintln(w)

	if len(c.commands) > 0 {
		fmt.Fprintln(w, "\nAvailable Commands:")
		sorted := make([]*Command, len(c.commands))
		copy(sorted, c.commands)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name() < sorted[j].Name() })

		width := 0
		for _, s := range sorted {
			if l := len(s.Name()); l > width {
				width = l
			}
		}
		for _, s := range sorted {
			fmt.Fprintf(w, "  %-*s  %s\n", width, s.Name(), s.Short)
		}
	}

	fmt.Fprintf(w, "\nUse \"%s [command] --help\" for more information about a command.\n", c.commandPath())
}
