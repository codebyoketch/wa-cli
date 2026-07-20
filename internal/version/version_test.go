package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestGet_ReflectsCurrentPackageVars(t *testing.T) {
	// Save and restore, since these are package-level vars ldflags would
	// normally set at build time — tests shouldn't leak overrides into
	// each other or into whatever ran before them.
	origVersion, origCommit, origDate := Version, Commit, BuildDate
	t.Cleanup(func() { Version, Commit, BuildDate = origVersion, origCommit, origDate })

	Version, Commit, BuildDate = "v1.2.3", "abc1234", "2026-01-01T00:00:00Z"

	info := Get()
	if info.Version != "v1.2.3" || info.Commit != "abc1234" || info.BuildDate != "2026-01-01T00:00:00Z" {
		t.Fatalf("Get() = %+v, want values matching the package vars set above", info)
	}
	if info.GoOS != runtime.GOOS || info.GoArch != runtime.GOARCH {
		t.Fatalf("Get() GoOS/GoArch = %s/%s, want %s/%s", info.GoOS, info.GoArch, runtime.GOOS, runtime.GOARCH)
	}
}

func TestInfo_String_ContainsAllFields(t *testing.T) {
	info := Info{
		Version:   "v1.2.3",
		Commit:    "abc1234",
		BuildDate: "2026-01-01T00:00:00Z",
		GoOS:      "linux",
		GoArch:    "amd64",
	}
	s := info.String()

	for _, want := range []string{"v1.2.3", "abc1234", "2026-01-01T00:00:00Z", "linux", "amd64"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() = %q, missing %q", s, want)
		}
	}
}
