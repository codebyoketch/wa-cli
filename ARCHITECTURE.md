# Architecture

This document is a map of wa-cli's codebase: how the packages fit
together and why they're split the way they are. For what's built vs.
planned, see [ROADMAP.md](./ROADMAP.md). For end-user usage, see
[README.md](./README.md).

## Overview

```
main.go
  └── cmd/                  Cobra command tree (one file per command/group)
        └── internal/app    Shared dependencies handed to every command
              ├── internal/config     Load/save config.json
              └── internal/logger     Structured logging (slog)

        └── internal/whatsapp        whatsmeow wrapper: the actual WhatsApp client
              ├── internal/store       Session/device persistence (SQLite)
              ├── internal/chatstore   Local chat index (JSON)
              ├── internal/msgstore    Local per-chat message history (JSON)
              └── internal/notify      Desktop notifications (beeep)

        └── internal/tui              Full-screen terminal UI (Bubble Tea)
        └── internal/extension        Plugin system (git-cloned subprocesses)
        └── internal/qr               ASCII QR rendering for login
        └── internal/ratelimit        Persisted send-rate limits
        └── internal/safety           Send guardrails (confirm-new-recipient, etc.)
        └── internal/errors           Shared error wrapping
        └── internal/version          Build-time version metadata
```

`main.go` is a two-line entrypoint that hands off to `cmd.Execute()`
immediately — all real logic lives under `cmd/` and `internal/`.

## `cmd/` — the command tree

One file per command or command group (`chat.go`, `contact.go`,
`group.go`, `media.go`, `config.go`, `extension.go`, `completion.go`,
`login.go`, `logout.go`, `status.go`, `send.go`/`reply.go`/`forward.go`,
`watch.go`, `version.go`, `root.go`), each self-registering onto
`rootCmd` from its own `init()`. `root.go` builds the shared `app.App`
(`var a *app.App`) once at package init time, before any command's
`RunE` runs, so every command can assume `a.Config`/`a.Log` are ready.

A few cross-cutting helpers live alongside the commands rather than in
`internal/` because they're specific to the CLI layer, not reusable
library code:

- **`cmd/json.go`** — the `--json` flag, `useJSON(cmd)` (flag overrides
  the `jsonOutput` config default for one call), and `printJSON`, the
  single encoder every JSON-mode command funnels through.
- **`cmd/stdlog.go`** — `captureLibraryStdout(fn)`, which redirects
  `os.Stdout` to `os.Stderr` for the duration of any whatsmeow call.
  whatsmeow's own logger (`waLog.Stdout`, see `internal/whatsapp`) writes
  directly to the process-wide stdout with no injectable writer; without
  this, a stray log line can land ahead of or inside a command's JSON
  output and break anything parsing it (`wa chat list --json | jq`).
  `internal/tui` hit the same problem and solved it the same way
  (redirect to a log file instead, since the TUI owns the whole screen).
- **`cmd/send_shared.go`** — the resolve-recipient / rate-limit /
  confirm-new-recipient logic shared by `chat send`, `chat reply`,
  `chat forward`, and `media send`.

## `internal/app` — shared dependencies

Deliberately tiny: loads config, builds a logger, returns an `*App`.
Notably, a broken config file (invalid JSON) does **not** abort startup
— `app.New()` warns to stderr and falls back to in-memory defaults, so
`wa config edit` (the one command that can fix a broken config file)
stays runnable even when the file it needs to fix is the thing that's
broken. Hard-failing here would strand anyone with a broken config with
no command that could rescue them.

## `internal/whatsapp` — the WhatsApp client

Wraps whatsmeow (`go.mau.fi/whatsmeow`), exposing a smaller, wa-cli-
shaped API (`Client.ListChats`, `SendImage`, `GroupInfo`, etc.) rather
than leaking whatsmeow's types everywhere. This is the one package that
talks to WhatsApp's servers; everything else in `cmd/` goes through it.

`SetNotifications` is called explicitly by `wa watch` and the TUI only
— not by one-shot commands that briefly construct a client (`chat list`,
`contact list`, ...) — so a five-second `chat list` sync never pops a
desktop notification.

## Local state: three separate stores, three separate reasons

wa-cli keeps three distinct pieces of local state, each for a different
reason — they're not interchangeable, and merging them would lose the
property each one exists for:

- **`internal/store`** — whatsmeow's own SQLite device/session store.
  This is what makes `wa login` persist across runs; whatsmeow owns its
  schema, wa-cli just points it at a data directory.
- **`internal/chatstore`** — a local JSON index of chats (name, JID,
  last message preview, unread count, mute state) so `wa chat
  list/search/info` have something to read *without* a network
  connection, across separate CLI invocations. Deliberately not a cache
  of everything WhatsApp knows — see the `HistorySync`-timing caveat in
  ROADMAP's Known Issues.
- **`internal/msgstore`** — a rolling window of recent messages per
  chat, so `wa chat open` can show numbered history and `wa chat
  reply`/`forward`/`media download` can reference a message by that
  number later, in a separate process invocation.

## `internal/tui` — the full-screen interface

Built on Bubble Tea/Lip Gloss/Bubbles. Runs when `wa` is invoked with no
subcommand. Owns the whole terminal for its run, which is why it needs
its own stdout redirect (to a log file, not stderr like
`captureLibraryStdout` — there's no "interactive terminal" to print
warnings to while the TUI has the screen).

## `internal/extension` — the plugin system

Extensions are plain git repositories with a `wa-extension.json`
manifest (name/description/entrypoint), cloned into
`extensions/<name>/` under the config directory and run as ordinary
subprocesses — not Go plugins (`.so` files need an exact toolchain match
and don't exist on Windows at all, which would break the cross-platform
goal from Phase 0). `Install` clones to a temp directory first and
validates the manifest/entrypoint before ever touching the real
extensions directory, and rejects path traversal in both `name` and
`entrypoint`.

## Supporting packages

- **`internal/config`** — loads/saves `config.json` from
  `$XDG_CONFIG_HOME/wa/` (or platform equivalent). `cmd/config.go`'s
  `configFields` table (name, typed getter, validating setter) is the
  single source of truth for every setting `get`/`set` know about, so
  the two commands can't drift out of sync with each other or with the
  `Config` struct itself.
- **`internal/ratelimit`** / **`internal/safety`** — send-side
  guardrails (`maxMessagesPerMinute/Hour/Day`, confirm-before-messaging-
  a-new-recipient) so a scripting mistake — a bad loop, a typo'd contact
  list — can't turn wa-cli into an accidental bulk-messaging tool.
  `ratelimit` persists its counters to disk specifically because each
  `wa` invocation is a short-lived process; an in-memory-only limiter
  would reset on every single command.
- **`internal/qr`** — renders WhatsApp's pairing code as an ASCII QR
  code for `wa login`.
- **`internal/errors`** — thin wrapping helpers (`Wrap`/`Wrapf`) around
  the standard library's `errors` package, used for consistent
  "context: underlying error" messages across the codebase.
- **`internal/logger`** — thin wrapper around `log/slog`, configured
  from `Config.LogLevel`/`Config.JSONOutput`.
- **`internal/version`** — build-time metadata (version/commit/date),
  overridden via `-ldflags` at build time; backs `wa version`.

## Dead code

**`internal/cli`** is a hand-rolled command router built early on,
before Cobra's dependencies were reachable in the original build
environment (see the note at the bottom of ROADMAP.md). Nothing imports
it anymore — `cmd/` has used real `github.com/spf13/cobra` since. It's
still sitting in the tree and is a reasonable candidate for deletion
whenever someone's doing cleanup; noted here rather than silently
removed, since it's not this document's place to make that call.
