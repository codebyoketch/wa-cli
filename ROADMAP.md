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
- [ ] **Phase 4 — Sending Messages**: text, emoji, mentions, reply, forward.
- [ ] **Phase 5 — Receiving Messages**: event handler, `wa watch`, read
      receipts, typing indicators.
- [ ] **Phase 6 — Contacts**: list, info, search.
- [ ] **Phase 7 — Groups**: list, create, add, remove, info.
- [ ] **Phase 8 — Media**: send/download/list images, video, audio,
      documents, stickers.
- [ ] **Phase 9 — Terminal UI**: Bubble Tea / Lip Gloss / Bubbles full-screen
      chat UI.
- [ ] **Phase 10 — Notifications**: desktop + terminal notifications,
      unread badge.
- [ ] **Phase 11 — Configuration**: `wa config set/get/edit`.
- [ ] **Phase 12 — Plugins**: `wa extension install/list/remove`.
- [ ] **Phase 13 — Shell Completion**: bash, zsh, fish, PowerShell.
- [ ] **Phase 14 — JSON Output**: `--json` across all list/read commands.
- [ ] **Phase 15 — Testing**: unit + integration tests, mock WhatsApp
      service, CI coverage (target 80%+ on core packages).
- [ ] **Phase 16 — Documentation**: site, examples, API docs, architecture
      diagrams.
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
