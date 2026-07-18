// Package version exposes build-time metadata for wa-cli.
//
// Values are overridden at build time via -ldflags, e.g.:
//
//	go build -ldflags "\
//	  -X github.com/codebyoketch/wa-cli/internal/version.Version=v0.1.0 \
//	  -X github.com/codebyoketch/wa-cli/internal/version.Commit=$(git rev-parse --short HEAD) \
//	  -X github.com/codebyoketch/wa-cli/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the semantic version of the build (set via ldflags).
	Version = "dev"
	// Commit is the git commit hash of the build (set via ldflags).
	Commit = "none"
	// BuildDate is the UTC build timestamp (set via ldflags).
	BuildDate = "unknown"
)

// Info is a snapshot of build metadata.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
	GoOS      string `json:"goos"`
	GoArch    string `json:"goarch"`
}

// Get returns the current build info.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoOS:      runtime.GOOS,
		GoArch:    runtime.GOARCH,
	}
}

// String renders a human-readable one-liner, e.g. "wa version v0.1.0 (commit abc123, built 2026-07-18T00:00:00Z)".
func (i Info) String() string {
	return fmt.Sprintf("wa version %s (commit %s, built %s, %s/%s)",
		i.Version, i.Commit, i.BuildDate, i.GoOS, i.GoArch)
}
