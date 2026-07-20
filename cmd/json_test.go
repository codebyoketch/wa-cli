package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/codebyoketch/wa-cli/internal/app"
	"github.com/codebyoketch/wa-cli/internal/config"
)

// newTestCmd builds a minimal cobra.Command with its own --json flag,
// independent of the real rootCmd tree. The flag is bound to the same
// package-level jsonOutput var real rootCmd binds to — that's what
// useJSON actually reads, so the test flag has to point at it the same
// way, or Changed("json") comes back true while useJSON reads a
// jsonOutput that was never touched.
func newTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "test"}
	c.Flags().BoolVar(&jsonOutput, "json", false, "")
	return c
}

// withApp temporarily swaps the package-level `a` (normally set once by
// rootCmd's init()) for the duration of the test, so useJSON's config
// fallback is exercised deterministically instead of depending on
// whatever's in the machine's real config file.
func withApp(t *testing.T, cfg config.Config) {
	t.Helper()
	orig := a
	a = &app.App{Config: cfg}
	t.Cleanup(func() { a = orig })
}

func TestUseJSON_FlagOverridesConfig_True(t *testing.T) {
	withApp(t, config.Config{JSONOutput: false})
	c := newTestCmd()
	if err := c.Flags().Set("json", "true"); err != nil {
		t.Fatalf("setting flag: %v", err)
	}

	if !useJSON(c) {
		t.Error("useJSON should return true when --json is explicitly set, even with jsonOutput=false in config")
	}
}

func TestUseJSON_FlagOverridesConfig_False(t *testing.T) {
	withApp(t, config.Config{JSONOutput: true})
	c := newTestCmd()
	if err := c.Flags().Set("json", "false"); err != nil {
		t.Fatalf("setting flag: %v", err)
	}

	if useJSON(c) {
		t.Error("useJSON should return false when --json=false is explicitly set, even with jsonOutput=true in config")
	}
}

func TestUseJSON_FallsBackToConfig_WhenFlagUntouched(t *testing.T) {
	withApp(t, config.Config{JSONOutput: true})
	c := newTestCmd() // flag never Set(), so Changed("json") is false

	if !useJSON(c) {
		t.Error("useJSON should fall back to config's JSONOutput=true when --json wasn't passed")
	}

	withApp(t, config.Config{JSONOutput: false})
	c2 := newTestCmd()
	if useJSON(c2) {
		t.Error("useJSON should fall back to config's JSONOutput=false when --json wasn't passed")
	}
}

func TestPrintJSON_IndentedNoHTMLEscape(t *testing.T) {
	c := newTestCmd()
	var buf bytes.Buffer
	c.SetOut(&buf)

	type payload struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := printJSON(c, payload{Name: "a & b", URL: "https://example.com?a=1&b=2"}); err != nil {
		t.Fatalf("printJSON: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "  \"name\"") {
		t.Errorf("printJSON output not indented as expected:\n%s", out)
	}
	if strings.Contains(out, "\\u0026") {
		t.Errorf("printJSON should not HTML-escape '&' (breaks readability for CLI/jq use):\n%s", out)
	}
	if !strings.Contains(out, "a & b") {
		t.Errorf("printJSON output missing expected literal '&':\n%s", out)
	}
}
