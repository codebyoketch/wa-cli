// Package extension implements wa-cli's plugin system.
//
// An extension is a git repository containing a manifest file
// (wa-extension.json) plus a single executable entrypoint. Extensions run
// as ordinary subprocesses — deliberately not Go plugins (.so), since
// those require the exact same Go toolchain/version as wa-cli itself and
// don't work on Windows at all, which would break Phase 0's
// cross-platform milestone. A subprocess model is the same approach
// git, kubectl, and gh all use for their own plugin systems.
//
// Installed extensions live under $XDG_CONFIG_HOME/wa/extensions/<name>,
// one directory per extension, named after the manifest's "name" field
// (not the repo URL) so re-installing from a fork or a renamed repo
// still lands in the same place.
package extension

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/codebyoketch/wa-cli/internal/config"
	waerrors "github.com/codebyoketch/wa-cli/internal/errors"
	"github.com/codebyoketch/wa-cli/internal/version"
)

// ManifestFile is the name of the manifest every extension repo must
// contain at its root.
const ManifestFile = "wa-extension.json"

// Manifest describes an extension. It's committed to the extension's own
// repo as wa-extension.json.
type Manifest struct {
	// Name is the extension's identifier — what users type after
	// `wa extension run`, `wa extension remove`, etc. Must be a single
	// path-safe path segment (no "/", no "..").
	Name string `json:"name"`
	// Description is a one-line summary shown by `wa extension list`.
	Description string `json:"description"`
	// Entrypoint is the executable to run, as a path relative to the
	// repo root (e.g. "wa-hello" or "bin/run.sh"). Must resolve inside
	// the repo — ".." components are rejected.
	Entrypoint string `json:"entrypoint"`
	// Version is an optional free-form version string, shown by
	// `wa extension list` when present. Purely informational — wa-cli
	// doesn't act on it.
	Version string `json:"version,omitempty"`
}

func (m Manifest) validate() error {
	if m.Name == "" {
		return waerrors.New("manifest missing \"name\"")
	}
	if m.Name != filepath.Base(m.Name) || m.Name == "." || m.Name == ".." {
		return waerrors.New("manifest \"name\" must be a plain identifier, not a path")
	}
	if m.Entrypoint == "" {
		return waerrors.New("manifest missing \"entrypoint\"")
	}
	clean := filepath.Clean(m.Entrypoint)
	if clean != m.Entrypoint || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return waerrors.New("manifest \"entrypoint\" must be a relative path inside the repo")
	}
	return nil
}

// Extension is an installed extension: its manifest plus where it lives
// on disk.
type Extension struct {
	Manifest
	// Path is the extension's install directory (repo root).
	Path string
}

// entrypointPath returns the absolute path to the extension's entrypoint
// executable.
func (e Extension) entrypointPath() string {
	return filepath.Join(e.Path, e.Entrypoint)
}

// Dir returns the directory extensions are installed into, creating it
// if needed.
func Dir() (string, error) {
	cfgDir, err := config.Dir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cfgDir, "extensions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", waerrors.Wrap(err, "creating extensions dir")
	}
	return dir, nil
}

// Install clones the git repo at source and registers it as an
// extension. source is passed straight to `git clone`, so anything git
// itself accepts works: https://..., git@..., or a local path — mainly
// useful for developing an extension before publishing it.
//
// The repo is cloned to a temporary directory first so its manifest can
// be validated before anything touches the real extensions dir; a
// broken or malicious extension never gets to overwrite an existing
// install.
func Install(source string) (*Extension, error) {
	if strings.TrimSpace(source) == "" {
		return nil, waerrors.New("extension source must not be empty")
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, waerrors.New("git not found in PATH — required to install extensions")
	}

	extDir, err := Dir()
	if err != nil {
		return nil, err
	}

	tmp, err := os.MkdirTemp(extDir, ".install-*")
	if err != nil {
		return nil, waerrors.Wrap(err, "creating temp dir")
	}
	defer os.RemoveAll(tmp)

	clonePath := filepath.Join(tmp, "repo")
	cloneCmd := exec.Command("git", "clone", "--depth=1", source, clonePath)
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr
	if err := cloneCmd.Run(); err != nil {
		return nil, waerrors.Wrapf(err, "cloning %s", source)
	}

	manifest, err := readManifest(clonePath)
	if err != nil {
		return nil, err
	}

	entrypointAbs := filepath.Join(clonePath, manifest.Entrypoint)
	if info, err := os.Stat(entrypointAbs); err != nil {
		return nil, waerrors.Wrapf(err, "entrypoint %q not found in %s", manifest.Entrypoint, source)
	} else if info.IsDir() {
		return nil, waerrors.Wrapf(waerrors.New("is a directory"), "entrypoint %q", manifest.Entrypoint)
	}
	// Best-effort: make sure the entrypoint is executable. Ignored on
	// platforms/filesystems where chmod isn't meaningful (e.g. Windows).
	_ = os.Chmod(entrypointAbs, 0o755)

	finalPath := filepath.Join(extDir, manifest.Name)
	if _, err := os.Stat(finalPath); err == nil {
		return nil, waerrors.New(fmt.Sprintf("extension %q is already installed (run 'wa extension remove %s' first to reinstall)", manifest.Name, manifest.Name))
	}

	if err := os.Rename(clonePath, finalPath); err != nil {
		return nil, waerrors.Wrap(err, "installing extension")
	}

	return &Extension{Manifest: manifest, Path: finalPath}, nil
}

// readManifest loads and validates wa-extension.json from repoPath.
func readManifest(repoPath string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(repoPath, ManifestFile))
	if os.IsNotExist(err) {
		return Manifest{}, waerrors.New(fmt.Sprintf("no %s found at the repo root — every wa-cli extension needs one", ManifestFile))
	}
	if err != nil {
		return Manifest{}, waerrors.Wrap(err, "reading manifest")
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, waerrors.Wrap(err, fmt.Sprintf("parsing %s", ManifestFile))
	}
	if err := m.validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// List returns every installed extension, sorted by name. An extension
// whose manifest is missing or invalid is skipped rather than failing
// the whole call — it's reported back via the errs slice so callers can
// warn about it without losing the rest of a valid list.
func List() (exts []Extension, errs []error) {
	dir, err := Dir()
	if err != nil {
		return nil, []error{err}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, []error{waerrors.Wrap(err, "reading extensions dir")}
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		manifest, err := readManifest(path)
		if err != nil {
			errs = append(errs, waerrors.Wrapf(err, "extension in %s", entry.Name()))
			continue
		}
		exts = append(exts, Extension{Manifest: manifest, Path: path})
	}
	return exts, errs
}

// Get returns the installed extension named name.
func Get(name string) (*Extension, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, waerrors.New(fmt.Sprintf("no extension named %q installed (see 'wa extension list')", name))
	}
	manifest, err := readManifest(path)
	if err != nil {
		return nil, err
	}
	return &Extension{Manifest: manifest, Path: path}, nil
}

// Remove uninstalls the extension named name.
func Remove(name string) error {
	ext, err := Get(name)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(ext.Path); err != nil {
		return waerrors.Wrap(err, "removing extension")
	}
	return nil
}

// Run executes the named extension as a subprocess with args, wiring up
// stdin/stdout/stderr directly so it behaves like any other terminal
// program (interactive prompts, colored output, etc. all work).
//
// wa-cli itself is exposed to the extension via the WA_CLI_VERSION
// environment variable, in case an extension wants to gate behavior on
// it; extensions otherwise run with no special access to wa-cli's own
// session, config, or store beyond what's already visible to any
// process running as the same user.
func Run(name string, args []string) error {
	ext, err := Get(name)
	if err != nil {
		return err
	}

	c := exec.Command(ext.entrypointPath(), args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = append(os.Environ(), "WA_CLI_VERSION="+version.Version)

	if err := c.Run(); err != nil {
		var exitErr *exec.ExitError
		if waerrors.As(err, &exitErr) {
			// The extension ran and exited non-zero on its own — that's
			// its business, not an error in wa-cli. Propagate the exit
			// code's meaning without wrapping it as a wa-cli failure.
			return waerrors.New(fmt.Sprintf("extension %q exited with an error", name))
		}
		return waerrors.Wrapf(err, "running extension %q", name)
	}
	return nil
}