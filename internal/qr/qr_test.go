package qr

import (
	"io"
	"os"
	"strings"
	"testing"
)

// TestPrint_WritesToStdout captures os.Stdout while Print runs and checks
// something non-trivial actually got written. It intentionally doesn't
// assert on the exact QR pattern (that's qrterminal's concern, not ours) —
// just that Print does what its doc comment promises: write an ASCII QR
// code for the given string to stdout.
func TestPrint_WritesToStdout(t *testing.T) {
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	Print("1@abc123,def456,ghi789==,jkl012==")

	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe writer: %v", err)
	}
	os.Stdout = orig

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}

	got := string(out)
	if strings.TrimSpace(got) == "" {
		t.Fatal("expected Print to write non-empty QR output to stdout, got nothing")
	}
	// A real QR code render is many lines of block characters — a
	// one-liner would indicate something silently broke.
	if lines := strings.Count(got, "\n"); lines < 5 {
		t.Errorf("expected a multi-line QR render, got only %d newline(s)", lines)
	}
}