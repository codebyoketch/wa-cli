// Package logger configures wa-cli's structured logger.
//
// It's a thin wrapper around log/slog so the rest of the codebase depends
// on this package (and can be swapped later) rather than on slog directly.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Options configures logger construction.
type Options struct {
	// Level is one of "debug", "info", "warn", "error". Defaults to "info".
	Level string
	// JSON switches to JSON output (useful for `--json` / automation mode).
	JSON bool
}

// New builds a *slog.Logger writing to stderr, so stdout stays clean for
// command output (important for `--json` piping).
func New(opts Options) *slog.Logger {
	level := parseLevel(opts.Level)

	handlerOpts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if opts.JSON {
		handler = slog.NewJSONHandler(os.Stderr, handlerOpts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, handlerOpts)
	}

	return slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
