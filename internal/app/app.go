// Package app wires together wa-cli's shared dependencies (config, logger).
// The command tree itself lives in package cmd, which depends on this
// package rather than the other way around — keeping main.go / cmd thin
// wrappers around app-level dependencies.
package app

import (
	"log/slog"

	"github.com/codebyoketch/wa-cli/internal/config"
	"github.com/codebyoketch/wa-cli/internal/logger"
)

// App holds shared dependencies handed to every command.
type App struct {
	Config config.Config
	Log    *slog.Logger
}

// New loads config and builds a logger from it.
func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	log := logger.New(logger.Options{Level: cfg.LogLevel, JSON: cfg.JSONOutput})
	return &App{Config: cfg, Log: log}, nil
}
