package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codebyoketch/wa-cli/internal/config"
)

func withTempConfigHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestNew_NoConfigFile_UsesDefaults(t *testing.T) {
	withTempConfigHome(t)

	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.Config != config.Default() {
		t.Fatalf("New().Config = %+v, want Default() = %+v", a.Config, config.Default())
	}
	if a.Log == nil {
		t.Fatal("New().Log is nil")
	}
}

// This is the behavior internal/app's own doc comment calls out
// explicitly: a broken config file must not take down every command,
// because 'wa config edit' — the one command that could fix it — needs
// to still run.
func TestNew_CorruptConfigFile_FallsBackToDefaultsWithoutError(t *testing.T) {
	withTempConfigHome(t)

	path, err := config.Path()
	if err != nil {
		t.Fatalf("config.Path: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("writing corrupt config: %v", err)
	}

	a, err := New()
	if err != nil {
		t.Fatalf("New() with a corrupt config file should not error (must leave 'wa config edit' runnable), got: %v", err)
	}
	if a.Config != config.Default() {
		t.Fatalf("New().Config with a corrupt file = %+v, want Default()", a.Config)
	}
}

func TestNew_ValidConfigFile_IsLoaded(t *testing.T) {
	withTempConfigHome(t)

	cfg := config.Default()
	cfg.LogLevel = "debug"
	cfg.MaxMessagesPerMinute = 7
	if err := config.Save(cfg); err != nil {
		t.Fatalf("config.Save: %v", err)
	}

	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.Config != cfg {
		t.Fatalf("New().Config = %+v, want %+v", a.Config, cfg)
	}
}

func TestNew_DataDirDefaultsUnderConfigDir(t *testing.T) {
	withTempConfigHome(t)

	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dir, err := config.Dir()
	if err != nil {
		t.Fatalf("config.Dir: %v", err)
	}
	if want := filepath.Join(dir, "data"); a.Config.DataDir != want {
		t.Errorf("DataDir = %q, want %q", a.Config.DataDir, want)
	}
}
