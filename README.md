# wa-cli

A WhatsApp client for your terminal, built in Go on top of [whatsmeow](https://github.com/tulir/whatsmeow).

> **Status:** actively developed, most core features working and verified
> against a real account — see [ROADMAP.md](./ROADMAP.md) for exactly
> what's done, what's in progress, and known issues.

## Why

Scriptable, fast, terminal-first WhatsApp: send and receive messages,
manage chats/contacts/groups/media, all from the command line — with
`--json` output for scripting and automation, and a full-screen TUI
(`wa`, no subcommand) for everyday use.

## Install

Requires Go 1.25+.

```sh
go install github.com/codebyoketch/wa-cli@latest
```

Or build from source:

```sh
git clone https://github.com/codebyoketch/wa-cli.git
cd wa-cli
make build   # binary at ./bin/wa
```

For quick local iteration without a build step, use `go run .` in place
of `wa` anywhere below.

## Getting started

```sh
wa login              # scan the QR code with WhatsApp on your phone
wa status             # confirm you're logged in
wa chat list           # sync and list your chats
```

Run `wa` with no subcommand to open the full-screen terminal UI instead
of using individual commands.

## Usage

**Chats**
```sh
wa chat list [--no-sync]        # list chats, most recent first
wa chat search <query>          # search chats by name
wa chat info <jid-or-name>      # details for one chat
wa chat open <jid-or-name>      # show local message history
wa chat send <recipient> <message...>
wa chat reply <recipient> <message-ref> <message...>
wa chat forward <from-chat> <message-ref> <to-recipient>
wa chat mute / unmute <chat>    # suppress desktop notifications per chat
```

**Receiving**
```sh
wa watch                        # long-running: print incoming messages live
```

**Contacts**
```sh
wa contact list
wa contact search <query>
wa contact info <name-or-jid>
```

**Groups**
```sh
wa group list
wa group info <name-or-jid>
wa group create <name> <participant...>
wa group add / remove <group> <participant...>
```

**Media**
```sh
wa media send image/video/audio/document/sticker <recipient> <file>
wa media download <chat> <message-ref> [output-path]
wa media list <chat>            # media messages in a chat's local history
```
`image` send/download are verified against a real account; the other
media types are implemented but not yet confirmed end-to-end — see
Phase 8 in [ROADMAP.md](./ROADMAP.md).

**Config**
```sh
wa config get [key]             # print all settings, or one
wa config set <key> <value>     # change and save one setting
wa config edit                  # open config.json in $EDITOR
wa config init                  # write out defaults
```

**Extensions**
```sh
wa extension install <git-url>
wa extension list
wa extension run <name> [-- args...]
wa extension remove <name>
```

**Other**
```sh
wa status                       # login status
wa version
wa completion bash|zsh|fish|powershell
```

### JSON output

Every list/read command above (`chat list/search/info/open`, `contact
list/search/info`, `group list/info`, `media list`, `status`, `extension
list`) supports `--json` for scripting:

```sh
wa chat list --json | jq '.[] | .name'
```

Set it as the default instead of passing the flag every time:

```sh
wa config set jsonOutput true
```

### Notifications

Desktop notifications fire from `wa watch` and the TUI when a new message
arrives (not from one-shot commands). Configurable via `wa config set`:

```sh
wa config set notifyEnabled true|false
wa config set notifyGroups true|false        # group chats opt-in separately from DMs
wa config set notifyShowPreview true|false   # include message text in the notification
wa chat mute <chat>                          # suppress notifications for one chat
```

## Development

```sh
make build   # compile ./bin/wa
make test    # go test ./...
make lint    # golangci-lint run
make fmt     # gofmt -w .
```

## Roadmap

See [ROADMAP.md](./ROADMAP.md) for the full phase-by-phase plan, what's
done, what's in progress, and known issues.

## Architecture

See [ARCHITECTURE.md](./ARCHITECTURE.md) for a map of the codebase —
package layout and how the pieces fit together.

## Examples

See [docs/EXAMPLES.md](./docs/EXAMPLES.md) for worked examples: scripting
with `--json`, a daily unread-chats digest, piping `wa watch`, sending
from a script without hitting an interactive prompt, exporting chat
history, and writing a minimal extension.

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md).

## Security

See [SECURITY.md](./SECURITY.md) for how to report vulnerabilities.

## License

[MIT](./LICENSE)
