package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// withTempConfigHome points os.UserConfigDir() (and therefore every
// function in this package) at a fresh temp directory for the test, the
// same way a real user's XDG_CONFIG_HOME scopes it.
func withTempConfigHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	return home
}

func TestDir_CreatesAndReturnsWaSubdir(t *testing.T) {
	home := withTempConfigHome(t)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if want := filepath.Join(home, "wa"); dir != want {
		t.Fatalf("Dir() = %q, want %q", dir, want)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("Dir() did not create the directory: %v", err)
	}
}

func TestPath_IsConfigJSONInsideDir(t *testing.T) {
	withTempConfigHome(t)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if want := filepath.Join(dir, "config.json"); path != want {
		t.Fatalf("Path() = %q, want %q", path, want)
	}
}

func TestDefault_HasSaneNonZeroValues(t *testing.T) {
	withTempConfigHome(t)

	cfg := Default()
	if cfg.LogLevel == "" {
		t.Error("Default().LogLevel is empty")
	}
	if cfg.DataDir == "" {
		t.Error("Default().DataDir is empty")
	}
	if !cfg.ConfirmNewRecipients {
		t.Error("Default().ConfirmNewRecipients should be true — this is a safety default, not an opt-in")
	}
	if cfg.MaxMessagesPerMinute <= 0 || cfg.MaxMessagesPerHour <= 0 || cfg.MaxMessagesPerDay <= 0 {
		t.Errorf("Default() rate limits should be positive by default, got %+v", cfg)
	}
}

func TestLoad_NoFileReturnsDefaults(t *testing.T) {
	withTempConfigHome(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with no file present should not error, got: %v", err)
	}
	if cfg != Default() {
		t.Fatalf("Load() with no file = %+v, want Default() = %+v", cfg, Default())
	}
}

func TestSaveThenLoad_RoundTrips(t *testing.T) {
	withTempConfigHome(t)

	cfg := Default()
	cfg.LogLevel = "debug"
	cfg.JSONOutput = true
	cfg.MaxMessagesPerMinute = 42
	cfg.NotifyGroups = false

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != cfg {
		t.Fatalf("round-tripped config = %+v, want %+v", got, cfg)
	}
}

func TestSave_WritesOwnerOnlyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits aren't meaningful on Windows")
	}
	withTempConfigHome(t)

	if err := Save(Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config.json permissions = %o, want 0600 — the config file may contain sensitive settings and shouldn't be group/world readable", perm)
	}
}

func TestLoad_CorruptFileReturnsError(t *testing.T) {
	withTempConfigHome(t)

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("writing corrupt config: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("Load() with a corrupt config file should return an error, not silently succeed")
	}
}
