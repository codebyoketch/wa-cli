// Package chatstore persists a local index of WhatsApp chats (JID, name,
// last message preview/time, unread count) so `wa chat` commands have
// something to read across separate CLI invocations, without needing an
// always-on connection.
//
// It's populated by internal/whatsapp's event handlers (message and
// history-sync events) and is intentionally a plain JSON file rather than
// a SQLite table: chat counts for a personal account are small (dozens to
// low hundreds), so there's no real performance need for a database here,
// and it keeps this package dependency-free and easy to unit test.
package chatstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Chat is one conversation's locally cached metadata.
type Chat struct {
	JID                string `json:"jid"`
	Name               string `json:"name"`
	IsGroup            bool   `json:"isGroup"`
	LastMessageAt      int64  `json:"lastMessageAt"` // unix millis, 0 if unknown
	LastMessagePreview string `json:"lastMessagePreview"`
	UnreadCount        int    `json:"unreadCount"`
}

// Store is a JSON-file-backed collection of Chats, keyed by JID.
type Store struct {
	path string
	mu   sync.Mutex
}

// New creates a Store whose state file lives under dataDir.
func New(dataDir string) *Store {
	return &Store{path: filepath.Join(dataDir, "chats.json")}
}

// Upsert inserts or updates a chat record. Fields left zero-valued on an
// update (e.g. an empty Name) do not overwrite existing non-empty values —
// callers can pass partial updates without first reading the record.
func (s *Store) Upsert(c Chat) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	chats, err := s.load()
	if err != nil {
		return err
	}

	existing, ok := chats[c.JID]
	if ok {
		if c.Name == "" {
			c.Name = existing.Name
		}
		if c.LastMessageAt == 0 {
			c.LastMessageAt = existing.LastMessageAt
			c.LastMessagePreview = existing.LastMessagePreview
		}
		if !c.IsGroup {
			c.IsGroup = existing.IsGroup
		}
	}

	chats[c.JID] = c
	return s.save(chats)
}

// Get returns the chat for jid, and whether it was found.
func (s *Store) Get(jid string) (Chat, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chats, err := s.load()
	if err != nil {
		return Chat{}, false, err
	}
	c, ok := chats[jid]
	return c, ok, nil
}

// List returns all chats sorted by most recent activity first. Chats with
// no known LastMessageAt sort last, alphabetically by name.
func (s *Store) List() ([]Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chats, err := s.load()
	if err != nil {
		return nil, err
	}

	out := make([]Chat, 0, len(chats))
	for _, c := range chats {
		out = append(out, c)
	}
	sortChats(out)
	return out, nil
}

// Search returns chats whose name or JID contains query, case-insensitive,
// sorted the same way as List.
func (s *Store) Search(query string) ([]Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chats, err := s.load()
	if err != nil {
		return nil, err
	}

	q := strings.ToLower(strings.TrimSpace(query))
	var out []Chat
	for _, c := range chats {
		if strings.Contains(strings.ToLower(c.Name), q) || strings.Contains(strings.ToLower(c.JID), q) {
			out = append(out, c)
		}
	}
	sortChats(out)
	return out, nil
}

// MarkRead zeroes the unread count for jid. No-op if jid isn't known.
func (s *Store) MarkRead(jid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	chats, err := s.load()
	if err != nil {
		return err
	}
	c, ok := chats[jid]
	if !ok {
		return nil
	}
	c.UnreadCount = 0
	chats[jid] = c
	return s.save(chats)
}

func sortChats(chats []Chat) {
	sort.Slice(chats, func(i, j int) bool {
		if chats[i].LastMessageAt != chats[j].LastMessageAt {
			return chats[i].LastMessageAt > chats[j].LastMessageAt
		}
		return strings.ToLower(chats[i].Name) < strings.ToLower(chats[j].Name)
	})
}

func (s *Store) load() (map[string]Chat, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return map[string]Chat{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading chat store: %w", err)
	}
	var chats map[string]Chat
	if err := json.Unmarshal(data, &chats); err != nil {
		return nil, fmt.Errorf("parsing chat store: %w", err)
	}
	if chats == nil {
		chats = map[string]Chat{}
	}
	return chats, nil
}

func (s *Store) save(chats map[string]Chat) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}
	data, err := json.MarshalIndent(chats, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding chat store: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("writing chat store: %w", err)
	}
	return nil
}
