# Contributing to wa-cli

Thanks for considering a contribution! wa-cli is developed one milestone at
a time — see [ROADMAP.md](./ROADMAP.md) for the current phase and what's
planned next. That's the best place to find work that fits where the
project is right now.

## Getting started

1. Fork and clone the repo.
2. Install Go 1.22+.
3. `make build && make test` to confirm everything works locally.
4. Pick an open issue, or a task from the current roadmap phase.

## Workflow

- Create a branch off `main`: `git checkout -b feature/short-description`.
- Keep commits focused; write clear commit messages (imperative mood, e.g.
  "Add chat search command").
- Run `make fmt lint test` before opening a PR.
- Update `CHANGELOG.md` under "Unreleased" for user-facing changes.
- Open a PR against `main` and describe what changed and why.

## Code style

- Standard `gofmt`/`go vet` cleanliness — enforced in CI.
- `golangci-lint run` must pass (config in `.golangci.yml`).
- Prefer small, focused packages under `internal/`, mirroring the existing
  structure (one concern per package).
- Commands live under their feature area's package and are wired into the
  command tree in `internal/app`.

## Tests

- New packages should ship with table-driven unit tests.
- `make test` runs `go test ./...`; CI also tracks coverage on core
  packages (target: 80%+, see Phase 15 in the roadmap).

## Reporting bugs / requesting features

Open a GitHub issue. For bugs, include your OS, Go version, and the exact
command you ran. For features, note which roadmap phase (if any) it fits
under, or propose it as a "Future (v2.0)" idea.

## Code of Conduct

Participation in this project is governed by our
[Code of Conduct](./CODE_OF_CONDUCT.md).
