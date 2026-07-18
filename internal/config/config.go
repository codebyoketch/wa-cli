// Package config loads and saves wa-cli's user configuration.
//
// The config file lives at $XDG_CONFIG_HOME/wa/config.json (falling back to
// ~/.config/wa/config.json). It's plain JSON for now rather than YAML/TOML
// so this package has zero third-party dependencies; a YAML front-end can
// be layered on top later without changing the on-disk struct.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	waerrors "github.com/codebyoketch/wa-cli/internal/errors"
)

// Config holds all user-configurable settings for wa-cli.
type Config struct {
	// LogLevel is one of "debug", "info", "warn", "error".
	LogLevel string `json:"logLevel"`
	// JSONOutput makes all commands default to `--json` formatting.
	JSONOutput bool `json:"jsonOutput"`
	// DataDir is where the SQLite session/device store lives.
	DataDir string `json:"dataDir"`
}

// Default returns sane defaults for a fresh install.
func Default() Config {
	dir, err := Dir()
	if err != nil {
		dir = "."
	}
	return Config{
		LogLevel:   "info",
		JSONOutput: false,
		DataDir:    filepath.Join(dir, "data"),
	}
}

// Dir returns the wa-cli config directory, creating it if needed.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", waerrors.Wrap(err, "resolving user config dir")
	}
	dir := filepath.Join(base, "wa")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", waerrors.Wrap(err, "creating config dir")
	}
	return dir, nil
}

// Path returns the full path to config.json.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config file, returning defaults (not an error) if it
// doesn't exist yet — first run shouldn't require `wa config init`.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, waerrors.Wrap(err, "reading config file")
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, waerrors.Wrap(err, "parsing config file")
	}
	return cfg, nil
}

// Save writes the config file, pretty-printed.
func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return waerrors.Wrap(err, "encoding config")
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return waerrors.Wrap(err, "writing config file")
	}
	return nil
}
