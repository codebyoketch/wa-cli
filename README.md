# wa-cli

A WhatsApp client for your terminal, built in Go on top of [whatsmeow](https://github.com/tulir/whatsmeow).

> **Status:** early development (Phase 0/1 — core architecture). Not yet usable
> for real messaging; see [ROADMAP.md](./ROADMAP.md) for what's done and what's next.

## Why

Scriptable, fast, terminal-first WhatsApp: list chats, send messages, watch
for new ones, all from the command line — with `--json` output for
automation, and an optional full-screen TUI down the line.

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

For quick local iteration without a build step:

```sh
go run . --help
```

## Usage

```sh
wa --help
wa version
wa config get
wa config init
```

WhatsApp login, chat, and messaging commands land in later phases — see the
roadmap.

## Development

```sh
make build   # compile ./bin/wa
make test    # go test ./...
make lint    # golangci-lint run
make fmt     # gofmt -w .
```

## Roadmap

See [ROADMAP.md](./ROADMAP.md) for the full phase-by-phase plan, from
project setup through v1.0.

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md).

## Security

See [SECURITY.md](./SECURITY.md) for how to report vulnerabilities.

## License

[MIT](./LICENSE)
