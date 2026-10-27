// Package msgstore persists a rolling window of recent messages per chat,
// so `wa chat open` can show numbered history and `wa chat reply`/
// `wa chat forward` can reference a specific message by that number or by
// its raw WhatsApp message ID.
//
// Like chatstore, this is a plain JSON file rather than SQLite — a
// personal account's recent message volume is small, and it keeps this
// package dependency-free and easy to unit test.
package msgstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// MaxPerChat is how many recent messages are always kept per chat; more
// are kept when needed to cover RetainFor.
const MaxPerChat = 50

// RetainFor is how far back every message is kept, even beyond MaxPerChat.
const RetainFor = 48 * time.Hour

// Message is one stored message.
type Message struct {
	ID        string `json:"id"`
	ChatJID   string `json:"chatJid"`
	SenderJID string `json:"senderJid"`
	Timestamp int64  `json:"timestamp"` // unix millis
	Text      string `json:"text"`      // caption, for media messages
	FromMe    bool   `json:"fromMe"`
	// MediaType is "" for plain text, or one of "image", "video",
	// "audio", "document", "sticker".
	MediaType string `json:"mediaType,omitempty"`
	// RawProto is the base64-encoded serialized WhatsApp message content,
	// used to reconstruct quote/forward context when replying or
	// forwarding, and to redownload media. Empty if unavailable
	// (unsupported message type, or encoding failed).
	RawProto string `json:"rawProto,omitempty"`
}

// Store is a JSON-file-backed per-chat message log.
type Store struct {
	path string
	mu   sync.Mutex
}

// New creates a Store whose state file lives under dataDir.
func New(dataDir string) *Store {
	return &Store{path: filepath.Join(dataDir, "messages.json")}
}

// Append adds msg to its chat's history, trimming it per trim.
func (s *Store) Append(msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.load()
	if err != nil {
		return err
	}
	all[msg.ChatJID] = trim(append(all[msg.ChatJID], msg), time.Now())
	return s.save(all)
}

// AppendMany merges msgs into their chats' histories with a single file
// read and write, skipping IDs already stored and keeping each chat in
// timestamp order (history syncs arrive out of order).
func (s *Store) AppendMany(msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.load()
	if err != nil {
		return err
	}
	touched := map[string]bool{}
	for _, msg := range msgs {
		if msg.ID != "" && hasID(all[msg.ChatJID], msg.ID) {
			continue
		}
		all[msg.ChatJID] = append(all[msg.ChatJID], msg)
		touched[msg.ChatJID] = true
	}
	now := time.Now()
	for jid := range touched {
		chatMsgs := all[jid]
		sort.SliceStable(chatMsgs, func(i, j int) bool { return chatMsgs[i].Timestamp < chatMsgs[j].Timestamp })
		all[jid] = trim(chatMsgs, now)
	}
	return s.save(all)
}

func hasID(msgs []Message, id string) bool {
	for _, m := range msgs {
		if m.ID == id {
			return true
		}
	}
	return false
}

// trim keeps the last MaxPerChat messages, plus any older ones still
// within RetainFor of now. msgs must be oldest first.
func trim(msgs []Message, now time.Time) []Message {
	if len(msgs) <= MaxPerChat {
		return msgs
	}
	cutoff := now.Add(-RetainFor).UnixMilli()
	keepFrom := len(msgs) - MaxPerChat
	for keepFrom > 0 && msgs[keepFrom-1].Timestamp >= cutoff {
		keepFrom--
	}
	return msgs[keepFrom:]
}

// List returns chatJID's stored messages, oldest first (matching the
// numbering `wa chat open` shows).
func (s *Store) List(chatJID string) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.load()
	if err != nil {
		return nil, err
	}
	return all[chatJID], nil
}

// Get returns the message with the given WhatsApp message id within
// chatJID's history, if present.
func (s *Store) Get(chatJID, id string) (Message, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.load()
	if err != nil {
		return Message{}, false, err
	}
	for _, m := range all[chatJID] {
		if m.ID == id {
			return m, true, nil
		}
	}
	return Message{}, false, nil
}

func (s *Store) load() (map[string][]Message, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return map[string][]Message{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading message store: %w", err)
	}
	var all map[string][]Message
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, fmt.Errorf("parsing message store: %w", err)
	}
	if all == nil {
		all = map[string][]Message{}
	}
	return all, nil
}

func (s *Store) save(all map[string][]Message) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding message store: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("writing message store: %w", err)
	}
	return nil
}
