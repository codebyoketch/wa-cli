package store

// NOTE: written without the ability to compile/run in the authoring
// sandbox — see the comment at the top of internal/whatsapp/client_test.go
// for why. Please run `go test ./internal/store/...` locally.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	waLog "go.mau.fi/whatsmeow/util/log"
)

func TestOpen_CreatesDataDirAndDBFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "data")
	log := waLog.Stdout("test", "ERROR", false)

	container, err := Open(context.Background(), dir, log)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if container == nil {
		t.Fatal("expected non-nil container")
	}

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected data dir to be created: %v", err)
	}

	dbPath := filepath.Join(dir, DBFileName)
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("expected %s to be created: %v", DBFileName, err)
	}
}

func TestOpen_ReopensExistingStore(t *testing.T) {
	dir := t.TempDir()
	log := waLog.Stdout("test", "ERROR", false)

	if _, err := Open(context.Background(), dir, log); err != nil {
		t.Fatalf("first Open: %v", err)
	}
	// Reopening the same dataDir should succeed against the existing
	// session.db rather than erroring out.
	if _, err := Open(context.Background(), dir, log); err != nil {
		t.Fatalf("second Open: %v", err)
	}
}
