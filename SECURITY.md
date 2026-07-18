# Security Policy

## Supported Versions

wa-cli is pre-1.0 and under active development. Until v1.0, only the latest
commit on `main` receives security fixes.

| Version      | Supported          |
| ------------ | ------------------- |
| `main` (dev) | :white_check_mark:  |
| < v1.0 tags  | :x:                  |

## Reporting a Vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Instead, email **oketchdishon@gmail.com** with:

- A description of the vulnerability and its potential impact
- Steps to reproduce (a minimal example helps a lot)
- Any suggested fix, if you have one

You should get an acknowledgment within a few days. Once a fix is ready,
we'll credit you in the release notes (unless you'd prefer to stay
anonymous).

## Scope

wa-cli talks to WhatsApp via the unofficial [whatsmeow](https://github.com/tulir/whatsmeow)
library and stores session data locally (SQLite). Particular areas of
interest for reports:

- Local storage of session/auth material (`internal/store`)
- Handling of QR/pairing codes (`internal/qr`)
- Any command that shells out or writes files outside the config/data dirs
