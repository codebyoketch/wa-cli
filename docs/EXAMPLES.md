# Examples

Worked, runnable examples beyond the one-liners in [README.md](../README.md).
Assumes `wa login` has already been run. Swap `wa` for `go run .` if
you're working from source without an installed binary.

## 1. Scripting basics: default to JSON everywhere

If you're going to script against wa-cli at all, set JSON as the default
once instead of adding `--json` to every call:

```sh
wa config set jsonOutput true
```

From here on, plain commands emit JSON, and `jq` becomes the natural way
to work with the output:

```sh
# Just names and unread counts
wa chat list | jq '.[] | {name, unreadCount}'

# Only chats with unread messages
wa chat list | jq '[.[] | select(.unreadCount > 0)]'

# A single group's member JIDs
wa group info "Family" | jq '.participants[].jid'
```

## 2. A daily unread-chats digest

A small script you could put on a cron job or a shell alias:

```sh
#!/usr/bin/env bash
set -euo pipefail

unread=$(wa chat list --json | jq '[.[] | select(.unreadCount > 0)]')
count=$(echo "$unread" | jq 'length')

if [ "$count" -eq 0 ]; then
  echo "No unread chats."
  exit 0
fi

echo "$count unread chat(s):"
echo "$unread" | jq -r '.[] | "  \(.name)  (\(.unreadCount) unread)  — \(.lastMessagePreview)"'
```

Note the explicit `--json` here rather than relying on the config
default — worth doing in any script you plan to share or run
unattended, so its behavior doesn't silently change if someone's local
`jsonOutput` config differs from yours.

## 3. Piping `wa watch` into another program

`wa watch` is a long-running foreground process, which makes it usable
as the input side of a pipeline — for example, logging every incoming
message to a file with a timestamp:

```sh
wa watch | while read -r line; do
  echo "$(date -Iseconds) $line" >> ~/wa-log.txt
done
```

(`wa watch`'s output isn't JSON-structured today — it's human-readable
lines. If you need structured events from `watch` specifically, that's
an open gap; see [ROADMAP.md](../ROADMAP.md).)

## 4. Sending from a script without an interactive prompt

By default, `confirmNewRecipients` makes wa-cli ask `[y/N]` before the
*first* message to any given number — a safeguard against a typo'd
contact list or a bad loop turning into an accidental bulk-send. That
prompt will hang a non-interactive script. Two ways to handle it:

**Turn it off entirely** (fine if you trust the script/recipient list):
```sh
wa config set confirmNewRecipients false
```

**Or message each recipient once interactively first**, so they're
already "known" the next time the script runs — `wa` remembers a
recipient as known after the first successful send, so subsequent runs
against the same numbers won't prompt again even with the guard left on.

Either way, the send-rate limits (`maxMessagesPerMinute/Hour/Day`) still
apply regardless — they're a separate guard, not tied to
`confirmNewRecipients`:

```sh
wa config set maxMessagesPerHour 50
```

## 5. Exporting a chat's local history to JSON

`wa chat open` also supports `--json`, returning both the chat's info
and its locally-known message history in one call — useful for a
personal backup/export script, keeping in mind it only covers what
wa-cli has synced locally (see the `HistorySync` caveat in
[ROADMAP.md](../ROADMAP.md)'s Known Issues), not your complete WhatsApp
history:

```sh
wa chat open "Mom" --json > mom-backup.json
jq '.messages | length' mom-backup.json
```

## 6. Writing a minimal extension

An extension is just a git repo with a manifest and one executable. This
is the smallest one that works — a shell script that echoes its
arguments:

```sh
mkdir wa-hello && cd wa-hello
git init

cat > wa-extension.json <<'EOF'
{
  "name": "wa-hello",
  "description": "Says hello",
  "entrypoint": "wa-hello.sh"
}
EOF

cat > wa-hello.sh <<'EOF'
#!/usr/bin/env sh
echo "Hello from an extension! Args: $*"
EOF
chmod +x wa-hello.sh

git add -A && git commit -m "initial"
```

Install and run it from anywhere (a local path works, so you don't need
to push it anywhere to try this out):

```sh
wa extension install /path/to/wa-hello
wa extension list
wa extension run wa-hello -- --loud
# → Hello from an extension! Args: --loud
```

A real extension would shell out to `wa` itself for WhatsApp data (e.g.
`wa chat list --json | jq ...` inside the script) rather than talking to
whatsmeow directly — extensions are subprocesses, not linked-in Go code,
so `wa`'s own JSON output is the integration point.

`entrypoint` must resolve to a path inside the repo (no `..`, no
absolute paths) and `name` must be a plain identifier, not a path —
`wa extension install` validates both before the extension is ever
copied into your real extensions directory, so a bad manifest fails
before it touches anything.
