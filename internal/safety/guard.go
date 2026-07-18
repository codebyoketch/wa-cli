// Package safety provides lightweight guardrails around outbound sending
// so wa-cli behaves like a normal personal WhatsApp client rather than a
// bulk-messaging tool. It's not a substitute for user judgment — it exists
// to stop a scripting mistake (a bad loop, a typo'd contact list) from
// silently messaging people you've never talked to.
package safety

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Guard tracks which recipient JIDs wa-cli has previously sent to or
// received a message from, so first-time sends to a new recipient can be
// flagged for explicit confirmation.
type Guard struct {
	path string
	mu   sync.Mutex
}

// New creates a Guard whose state file lives under dataDir.
func New(dataDir string) *Guard {
	return &Guard{path: filepath.Join(dataDir, "known_recipients.json")}
}

// IsKnown reports whether jid has been messaged (or has messaged us)
// before, according to local history.
func (g *Guard) IsKnown(jid string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	known, err := g.load()
	if err != nil {
		return false, err
	}
	return known[jid], nil
}

// MarkKnown records jid as a known recipient. Call this after a successful
// first send (or when a chat is opened/discovered), so later sends to the
// same JID don't require re-confirmation.
func (g *Guard) MarkKnown(jid string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	known, err := g.load()
	if err != nil {
		return err
	}
	if known[jid] {
		return nil
	}
	known[jid] = true
	return g.save(known)
}

func (g *Guard) load() (map[string]bool, error) {
	data, err := os.ReadFile(g.path)
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading known recipients: %w", err)
	}
	var known map[string]bool
	if err := json.Unmarshal(data, &known); err != nil {
		return nil, fmt.Errorf("parsing known recipients: %w", err)
	}
	if known == nil {
		known = map[string]bool{}
	}
	return known, nil
}

func (g *Guard) save(known map[string]bool) error {
	if err := os.MkdirAll(filepath.Dir(g.path), 0o700); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}
	data, err := json.Marshal(known)
	if err != nil {
		return fmt.Errorf("encoding known recipients: %w", err)
	}
	if err := os.WriteFile(g.path, data, 0o600); err != nil {
		return fmt.Errorf("writing known recipients: %w", err)
	}
	return nil
}
