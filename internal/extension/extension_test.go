package extension

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// newTestRepo creates a local git repository at a temp path containing
// the given manifest and a trivial shell entrypoint that exits 0. It
// returns the repo's path, suitable for passing straight to Install
// (git clone accepts local paths).
func newTestRepo(t *testing.T, manifestJSON string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test relies on a shell entrypoint and git CLI behavior not exercised on windows here")
	}

	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, ManifestFile), []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
	entrypoint := "#!/bin/sh\necho hello from extension\nexit 0\n"
	if err := os.WriteFile(filepath.Join(repo, "run.sh"), []byte(entrypoint), 0o755); err != nil {
		t.Fatalf("writing entrypoint: %v", err)
	}

	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-q", "-m", "initial")

	return repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// withTempExtensionsDir points config.Dir() (and therefore extension.Dir())
// at a fresh temp directory for the duration of the test, the same way a
// real user's XDG_CONFIG_HOME would scope it.
func withTempExtensionsDir(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestInstall_Simple(t *testing.T) {
	withTempExtensionsDir(t)
	repo := newTestRepo(t, `{"name":"hello","description":"says hi","entrypoint":"run.sh"}`)

	ext, err := Install(repo)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if ext.Name != "hello" || ext.Description != "says hi" {
		t.Fatalf("unexpected extension: %+v", ext)
	}
	if _, err := os.Stat(ext.entrypointPath()); err != nil {
		t.Fatalf("expected entrypoint on disk: %v", err)
	}
}

func TestInstall_MissingManifest(t *testing.T) {
	withTempExtensionsDir(t)
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	os.Mkdir(repo, 0o755)
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("no manifest here"), 0o644)
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-q", "-m", "initial")

	if _, err := Install(repo); err == nil {
		t.Fatal("expected error installing a repo with no manifest")
	}
}

func TestInstall_RejectsPathTraversalEntrypoint(t *testing.T) {
	withTempExtensionsDir(t)
	repo := newTestRepo(t, `{"name":"evil","description":"","entrypoint":"../../etc/passwd"}`)

	if _, err := Install(repo); err == nil {
		t.Fatal("expected error for entrypoint escaping repo root")
	}
}

func TestInstall_RejectsPathAsName(t *testing.T) {
	withTempExtensionsDir(t)
	repo := newTestRepo(t, `{"name":"../evil","description":"","entrypoint":"run.sh"}`)

	if _, err := Install(repo); err == nil {
		t.Fatal("expected error for a name that isn't a plain identifier")
	}
}

func TestInstall_DuplicateNameRejected(t *testing.T) {
	withTempExtensionsDir(t)
	repo := newTestRepo(t, `{"name":"hello","description":"","entrypoint":"run.sh"}`)

	if _, err := Install(repo); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	if _, err := Install(repo); err == nil {
		t.Fatal("expected error installing the same extension name twice")
	}
}

func TestList(t *testing.T) {
	withTempExtensionsDir(t)

	if exts, errs := List(); len(exts) != 0 || len(errs) != 0 {
		t.Fatalf("expected empty list before any installs, got exts=%v errs=%v", exts, errs)
	}

	repo := newTestRepo(t, `{"name":"hello","description":"says hi","entrypoint":"run.sh"}`)
	if _, err := Install(repo); err != nil {
		t.Fatalf("Install: %v", err)
	}

	exts, errs := List()
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(exts) != 1 || exts[0].Name != "hello" {
		t.Fatalf("unexpected list: %+v", exts)
	}
}

func TestRemove(t *testing.T) {
	withTempExtensionsDir(t)
	repo := newTestRepo(t, `{"name":"hello","description":"","entrypoint":"run.sh"}`)
	if _, err := Install(repo); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if err := Remove("hello"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := Get("hello"); err == nil {
		t.Fatal("expected Get to fail after Remove")
	}
}

func TestRemove_UnknownExtension(t *testing.T) {
	withTempExtensionsDir(t)
	if err := Remove("nope"); err == nil {
		t.Fatal("expected error removing an extension that was never installed")
	}
}

func TestRun(t *testing.T) {
	withTempExtensionsDir(t)
	repo := newTestRepo(t, `{"name":"hello","description":"","entrypoint":"run.sh"}`)
	if _, err := Install(repo); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if err := Run("hello", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRun_UnknownExtension(t *testing.T) {
	withTempExtensionsDir(t)
	if err := Run("nope", nil); err == nil {
		t.Fatal("expected error running an extension that was never installed")
	}
}
