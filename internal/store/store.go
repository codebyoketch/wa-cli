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
	// busy_timeout tells SQLite to wait (up to 5s) for a lock to clear
	// instead of failing immediately with SQLITE_BUSY — whatsmeow's
	// internal goroutines can touch the device store concurrently within
	// a single process (e.g. during logout's multi-table delete), and
	// the pure-Go sqlite driver doesn't set this by default.
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", dbPath)

	container, err := sqlstore.New(ctx, "sqlite", dsn, log)
	if err != nil {
		return nil, fmt.Errorf("opening session store: %w", err)
	}
	return container, nil
}
