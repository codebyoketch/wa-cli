# wa-cli Roadmap

Built one milestone at a time. Each phase leaves the project in a working
state; boxes get checked as milestones land. See `CHANGELOG.md` for the
detailed history.

- [x] **Phase 0 — Project Setup**: repo, license, docs, CI, lint config,
      Makefile, Cobra-style command entrypoint.
      **Milestone:** `wa --help` works on Windows, Linux, and macOS.
- [x] **Phase 1 — Core Architecture**: `internal/app`, `internal/config`,
      `internal/logger`, `internal/store`, `internal/version`,
      `internal/errors`.
      **Milestone:** `wa version`, `wa config`.
- [x] **Phase 2 — WhatsApp Authentication**: whatsmeow init, SQLite device
      store, QR login, logout, status, session persistence.
      **Milestone:** `wa login`, `wa logout`, `wa status` — verified against
      a real account, session persists across runs.
- [x] **Phase 3 — Chats**: `internal/chatstore` (local JSON index, survives
      across CLI invocations), `wa chat list/search/info/open`.
      `open` currently shows info + marks read; full message history is
      Phase 4/5 territory once sending/receiving exist.
- [x] **Phase 4 — Sending Messages**: `wa chat send/reply/forward` — text
      only (emoji works as plain UTF-8 text already; mentions not
      implemented). Verified end-to-end against a real account: send,
      reply (with quoted context), and forward (reconstructed from local
      msgstore) all confirmed landing on the recipient's phone.
- [x] **Phase 5 — Receiving Messages**: `wa watch` — long-running
      connection, prints incoming messages, reconnects on drops with
      backoff. Built ahead of Phase 4 (out of roadmap order) once
      connection-reliability testing showed a persistent, reconnecting
      connection was needed regardless — read receipts / typing
      indicators not yet implemented.
- [x] **Phase 6 — Contacts**: `wa contact list/search/info` — reads the
      local device store directly, no network connection needed.
- [x] **Phase 7 — Groups**: `wa group list/info/create/add/remove`.
      Verified against a real account — create confirmed working with a
      genuine third-party participant (a self-only participant list
      correctly gets rejected by WhatsApp's server, not a bug).
- [ ] **Phase 8 — Media**: send/download/list images, video, audio,
      documents, stickers. `wa media send image` and `wa media download`
      both verified against a real account. Found and fixed a real bug
      in the process: sendMedia was recording a fake text-only
      placeholder instead of the actual sent message, making anything
      sent via wa-cli silently undownloadable.
      Video/audio/document/sticker send not yet tested.
      documents, stickers.
- [x] **Phase 9 — Terminal UI**: `wa` (no subcommand) opens a split-pane
      chat UI (sidebar + messages + input), internal/tui, built on
      Bubble Tea/Lip Gloss/Bubbles. Verified working against a real
      account — including fixes for stdout log corruption, chat-name
      flip-flop, missing group names, and WhatsApp-style message
      alignment/sender names.
- [x] **Phase 10 — Notifications**: `internal/notify` (beeep) wraps OS
      desktop notifications; `SetNotifications` wires it in explicitly
      from `wa watch` and the TUI only, so one-shot commands that briefly
      touch the same client code path never pop a notification.
      Delivery is gated in `internal/whatsapp/client.go` on
      `notifyEnabled`, skips messages from self, respects
      `notifyGroups` (group chats opt-in separately from DMs), checks
      per-chat `Muted` via `chatstore` before sending, and — when
      `notifyShowPreview` is on — includes message text and/or media
      type in the body. All four settings
      (notifyEnabled/notifyGroups/notifyShowPreview, plus mute state)
      are configurable via `wa config set` and `wa chat mute/unmute`.
- [x] **Phase 11 — Configuration**: `wa config get/set/edit/init`.
      `configFields` is a single source-of-truth table (name, typed
      getter, validating setter) covering every config key — including
      the Phase 10 notify settings above — so `get` and `set` can't
      drift out of sync. `get` prints one key or the whole table with
      the config file path; `set` validates and saves immediately;
      `edit` opens `$EDITOR`/`$VISUAL`/`vi` on the JSON file and
      re-validates on exit, leaving the file untouched if the edit
      broke it; `init` writes out defaults.
- [x] **Phase 12 — Plugins**: `wa extension install/list/remove/run` —
      `internal/extension`, `cmd/extension.go`. Extensions are git repos
      with a `wa-extension.json` manifest (name/description/entrypoint)
      cloned into `extensions/<name>` under the config dir; they run as
      plain subprocesses (not Go plugins — `.so` requires an exact
      toolchain match and doesn't exist on Windows, which would break
      the Phase 0 cross-platform milestone). `install` clones to a temp
      dir first and validates the manifest/entrypoint before it ever
      touches the real extensions dir, rejects path-traversal in both
      `name` and `entrypoint`, and refuses to clobber an existing
      install. Verified against real Go 1.25: `go build ./...`/`go vet
      ./...` clean, all 9 unit tests pass (install/list/remove/run
      against local git repos, including path-traversal and
      duplicate-name rejection), and a manual end-to-end run — install
      a local `wa-hello` extension, `wa extension list`, `wa extension
      run wa-hello -- --loud` — round-tripped correctly, including args
      passed through to the subprocess.
- [x] **Phase 13 — Shell Completion**: `cmd/completion.go`, `wa completion
      bash/zsh/fish/powershell`. Static command-tree completion verified
      working end-to-end (built binary, sourced into bash, confirmed
      `wa <TAB>` and nested subcommand lists render correctly with
      descriptions). Dynamic completion described in `completion.go`'s
      own doc comment — chat names for `wa chat send/reply/forward`,
      contact names for `wa contact info`, config keys for `wa config
      get/set`, extension names for `wa extension run/remove` — is not
      yet implemented; no `ValidArgsFunction` exists anywhere in the
      source, so those commands currently fall back to default file
      completion. `wa group`/`wa media` intentionally use file-completion
      fallback per the one-active-connection constraint (see Known
      issues) — not a gap, by design.
- [x] **Phase 14 — JSON Output**: persistent `--json` flag (`cmd/json.go`)
      wired into every list/read command: `wa chat list/search/info/open`,
      `wa contact list/search/info`, `wa group list/info`, `wa media
      list`, `wa status`, `wa extension list`. `--json` overrides the
      `jsonOutput` config default in either direction for a single call;
      with neither set, falls back to `jsonOutput` from config (so
      scripted/agent use can set it once via `wa config set jsonOutput
      true` instead of passing `--json` everywhere). Empty results
      marshal as `[]`, never `null`. Added missing `json:"..."` tags to
      `whatsapp.Contact`/`Group`/`Participant` and
      `extension.Extension.Path` to match the camelCase convention
      `chatstore.Chat`/`msgstore.Message` already used.
      Verified against a real account — surfaced and fixed a real bug in
      the process: whatsmeow's `waLog.Stdout(...)` writes log lines
      directly to the process-wide `os.Stdout` with no injectable
      writer, so a WARN during `wa chat list --json`'s sync landed ahead
      of the JSON and broke `jq` parsing. Fixed with a shared
      `captureLibraryStdout` helper (`cmd/stdlog.go`) that redirects
      `os.Stdout` to `os.Stderr` for the duration of any whatsmeow call —
      same trick Phase 9's TUI already used for the same underlying
      problem, generalized for one-shot commands. Wired into
      `syncAndLoadChats`, `loadContacts`, `group list`/`info`, and
      `status`; `wa media list` and `wa extension list` don't touch
      whatsmeow directly so weren't at risk.
- [ ] **Phase 15 — Testing**: unit + integration tests, mock WhatsApp
      service, CI coverage (target 80%+ on core packages).
- [ ] **Phase 16 — Documentation**: site, examples, API docs, architecture
      diagrams. `README.md` rewritten to match actual capabilities
      (was still describing Phase 0/1 status), `ARCHITECTURE.md` added
      (package map, how `cmd`/`internal/app`/`internal/whatsapp` and the
      three local stores fit together, notes `internal/cli` as unused
      dead code), `docs/EXAMPLES.md` added (six worked examples:
      scripting with `--json`, a daily unread-chats digest, piping `wa
      watch`, sending from a script without hitting the
      confirm-new-recipient prompt, exporting chat history, writing a
      minimal extension). Still open: an actual hosted/generated docs
      site (what exists is markdown in the repo, not a site), generated
      API docs (godoc/pkg.go.dev-style — package doc comments are
      mostly there, but nothing's been generated or checked to render
      cleanly), and real architecture diagrams (ARCHITECTURE.md has an
      ASCII tree, not a diagram, e.g. Mermaid or an image).
- [ ] **Phase 17 — Releases**: GitHub Releases, Homebrew, Scoop, AUR,
      Docker image, `go install`, prebuilt binaries.
- [ ] **Phase 18 — v1.0**: stable, cross-platform, documented, tested.

## Future (v2.0 ideas)

- Multi-account support
- Message scheduling
- Chat backups
- Export to Markdown/HTML/PDF
- AI-powered search and summaries
- Plugin marketplace
- End-to-end encrypted local history index
- Remote mode (connect to a running `wa-cli` instance over SSH)

## Known issues

- **`wa chat list` is not a complete mirror of your WhatsApp inbox.**
  `HistorySync` only fires once, right after login, and WhatsApp decides
  what counts as "recent" — older, inactive chats may not appear until
  they get a new message. `wa chat list` reflects that one-time snapshot
  plus whatever's arrived live since (via `wa watch` or `chat list`'s own
  brief syncs), and gets more complete the longer `wa watch` has been
  running. True backfill of full history needs whatsmeow's on-demand
  history sync (explicitly requesting older data), which isn't
  implemented — scoped as a possible future addition to Phase 3, not
  currently planned.
- **`wa chat list`/`--json` sometimes shows an empty `name` for a chat
  that has one.** Seen on a real account: a group and a 1:1 chat both
  came back with `"name": ""` in `wa chat list --json` output, even
  though the chat clearly has a name in WhatsApp itself. Not yet
  root-caused — candidates are the same one-time-`HistorySync` timing
  gap noted above (name arrives in a later sync than the chat entry
  itself), or a gap specific to the JSON path vs. the human-readable
  `printChats` path, which hasn't been directly compared side-by-side
  on the same data yet. Needs reproduction with `wa chat list` (non-JSON)
  against the same chats to tell which it is.
- **Only one wa-cli connection can be active at a time, by WhatsApp's
  design.** WhatsApp allows one active connection per linked device.
  Running `wa chat list` (or any connecting command) while `wa watch` is
  running will disconnect `watch` — this isn't fixable client-side, it's
  how the protocol works. Use `wa chat list --no-sync` to read the local
  cache without opening a competing connection while `watch` is active.
- **Tab completion only covers the static command tree, not individual
  items.** `wa chat send`, `wa contact info`, `wa config get/set`, and
  `wa extension run/remove` fall back to default shell (file) completion
  instead of suggesting real chat names, contact names, config keys, or
  extension names — no `ValidArgsFunction` has been implemented for any
  of them yet, despite `cmd/completion.go`'s doc comment describing this
  as the intended behavior. Scoped to Phase 13.
- **Connection stability depends heavily on your network.** Testing
  surfaced frequent `Error sending close to websocket` resets on an
  Airtel 5G home connection, most likely due to CGNAT (carrier-grade
  NAT) common on cellular home broadband, which tends to apply short,
  aggressive timeouts to long-lived connections like WhatsApp's
  multi-device WebSocket. `wa watch`'s reconnect-with-backoff exists
  specifically to cope with this; one-shot commands (`chat list`,
  `login`) are more exposed to it, since they only get a short window
  before giving up.

## Notes on current implementation

Phases 0/1 originally used a small hand-rolled command router
(`internal/cli`) because the sandbox they were first built in couldn't
reach `proxy.golang.org` / `gopkg.in`, which Cobra's dependency graph
needs. That's since been swapped for real `github.com/spf13/cobra` — the
project now follows the standard `cobra-cli init` layout: `main.go` at the
repo root imports `cmd`, and `cmd/` holds one file per command/group
(`root.go`, `version.go`, `config.go`, `login.go`, `status.go`, `logout.go`,
`chat.go`), each self-registering onto `rootCmd` via `init()`.

Chat history sync (Phase 3) relies on whatsmeow's `*events.HistorySync`,
one of its more version-sensitive event types — the field names/shape in
`internal/whatsapp/client.go`'s `ingestHistorySync` were written against
the mainline API at the time and may need adjusting against whichever
whatsmeow version is actually pinned in `go.mod`.
