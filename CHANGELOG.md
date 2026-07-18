# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project intends to follow [Semantic Versioning](https://semver.org/)
from v1.0.0 onward.

## [Unreleased]

### Added

- Phase 0: project scaffold — MIT license, README, CONTRIBUTING,
  CODE_OF_CONDUCT, SECURITY, this changelog, GitHub Actions CI, golangci-lint
  config, Makefile.
- Phase 1: core architecture — `internal/config`, `internal/logger`,
  `internal/version`, `internal/errors`, `internal/app`, plus a minimal
  dependency-free `internal/cli` command router.
- `wa --help`, `wa version`, `wa config get`, `wa config init`.
