// wa-cli is a WhatsApp client for your terminal, built on
// go.mau.fi/whatsmeow. It links as a WhatsApp Web device (no browser
// needed) and gives you chats, contacts, groups, and media send/receive
// from the command line — plus a split-pane TUI (run wa with no
// subcommand), --json output for scripting, and a plugin system
// (wa extension).
//
// See README.md for installation and a full command list,
// ARCHITECTURE.md for how the pieces fit together, and
// docs/EXAMPLES.md for worked scripting examples. The actual command
// implementations live in the cmd package; this file just wires up
// Cobra's entrypoint.
package main

import "github.com/codebyoketch/wa-cli/cmd"

func main() {
	cmd.Execute()
}
