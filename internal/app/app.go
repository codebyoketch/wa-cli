// Package app wires together wa-cli's shared dependencies (config, logger).
// The command tree itself lives in package cmd, which depends on this
// package rather than the other way around — keeping main.go / cmd thin
// wrappers around app-level dependencies.
package app

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/codebyoketch/wa-cli/internal/config"
	"github.com/codebyoketch/wa-cli/internal/logger"
)

// App holds shared dependencies handed to every command.
type App struct {
	Config config.Config
	Log    *slog.Logger
}

// New loads config and builds a logger from it.
//
// A config file that fails to load (e.g. invalid JSON left behind by a
// half-finished 'wa config edit') does NOT abort startup. New() falls
// back to in-memory defaults instead, so every command keeps working —
// crucially including 'wa config edit' itself, which is the one command
// a user needs to actually fix the file. Hard-failing here would strand
// anyone with a broken config file with no command that could rescue
// them.
func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wa: warning: %v — using defaults for this run (run 'wa config edit' to fix the file)\n", err)
		cfg = config.Default()
	}
	log := logger.New(logger.Options{Level: cfg.LogLevel, JSON: cfg.JSONOutput})
	return &App{Config: cfg, Log: log}, nil
}
