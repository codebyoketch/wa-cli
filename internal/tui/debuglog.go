package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// debugFile is non-nil only when WA_DEBUG=1. Debug output must never go to
// stdout — that's Bubble Tea's screen.
var debugFile *os.File

func openDebugLog(dataDir string) {
	if os.Getenv("WA_DEBUG") != "1" {
		return
	}
	f, err := os.OpenFile(filepath.Join(dataDir, "wa-debug.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		debugFile = f
	}
}

func dlog(format string, args ...any) {
	if debugFile == nil {
		return
	}
	fmt.Fprintf(debugFile, "%s "+format+"\n", append([]any{time.Now().Format("15:04:05.000")}, args...)...)
}
