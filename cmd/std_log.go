package cmd

import "os"

// captureLibraryStdout temporarily redirects the process-wide os.Stdout
// to os.Stderr for the duration of fn, then restores it.
//
// whatsmeow's waLog.Stdout(...) loggers write directly to os.Stdout with
// no way to inject a different io.Writer — see
// https://pkg.go.dev/go.mau.fi/whatsmeow/util/log. Left alone, any log
// line it emits (even just a WARN during a routine sync) lands ahead of
// or interleaved with a command's own output. For a human that's just
// untidy; for anything reading stdout as data — `wa chat list --json |
// jq`, a script, an agent — a single stray log line breaks parsing
// outright. internal/tui hit the same problem and worked around it by
// swapping os.Stdout to a log file for its whole run; this is the same
// fix generalized for one-shot commands, redirecting to stderr instead
// of a file so the warning stays visible interactively while stdout
// itself stays clean.
func captureLibraryStdout(fn func() error) error {
	real := os.Stdout
	os.Stdout = os.Stderr
	defer func() { os.Stdout = real }()
	return fn()
}