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
	"sync"
)

// MaxPerChat caps how many recent messages are kept per chat.
const MaxPerChat = 50

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

// Append adds msg to its chat's history, trimming to the MaxPerChat most
// recent messages for that chat.
func (s *Store) Append(msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.load()
	if err != nil {
		return err
	}

	chatMsgs := append(all[msg.ChatJID], msg)
	if len(chatMsgs) > MaxPerChat {
		chatMsgs = chatMsgs[len(chatMsgs)-MaxPerChat:]
	}
	all[msg.ChatJID] = chatMsgs

	return s.save(all)
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
