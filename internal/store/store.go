// Package store manages wa-cli's local WhatsApp session/device data,
// backed by whatsmeow's SQLite device store.
package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"

	// Pure-Go SQLite driver — avoids requiring cgo/a C toolchain to build wa-cli.
	_ "modernc.org/sqlite"
)

// DBFileName is the SQLite file used for the whatsmeow device/session store.
const DBFileName = "session.db"

// Open opens (creating if needed) the whatsmeow SQLite device store rooted
// at dataDir/session.db.
func Open(ctx context.Context, dataDir string, log waLog.Logger) (*sqlstore.Container, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, DBFileName)
	// WAL (write-ahead logging) allows concurrent readers alongside a
	// single writer, unlike SQLite's default rollback journal mode —
	// important here because whatsmeow's initial sync does history-sync
	// media cleanup, app-state fetching, and incoming message writes
	// concurrently, and the default mode was hitting SQLITE_BUSY even
	// with a busy_timeout set. busy_timeout is still set as a fallback
	// for the cases WAL alone doesn't cover (e.g. two writers).
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)", dbPath)

	container, err := sqlstore.New(ctx, "sqlite", dsn, log)
	if err != nil {
		return nil, fmt.Errorf("opening session store: %w", err)
	}
	return container, nil
}
