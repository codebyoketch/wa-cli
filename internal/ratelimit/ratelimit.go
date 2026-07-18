// Package ratelimit enforces send-rate limits for wa-cli, persisted to disk
// so the limits hold across separate CLI invocations (each `wa` run is a
// short-lived process, so an in-memory-only limiter would reset every time).
//
// This exists to keep wa-cli behaving like a normal, personal WhatsApp
// client rather than a bulk-messaging tool: WhatsApp's abuse detection
// looks at send volume and velocity, not connection method, so a sane
// default ceiling protects the account even from an accidental loop or
// script bug, not just intentional misuse.
package ratelimit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Config sets the ceilings enforced by a Limiter. A zero value for any
// field means "no limit" for that window.
type Config struct {
	PerMinute int
	PerHour   int
	PerDay    int
}

// DefaultConfig returns conservative defaults suitable for personal use:
// well within what a normal person messaging on WhatsApp would ever hit,
// but low enough to stop a runaway script fast.
func DefaultConfig() Config {
	return Config{
		PerMinute: 10,
		PerHour:   100,
		PerDay:    500,
	}
}

// Limiter enforces Config against a persisted history of send timestamps.
type Limiter struct {
	path string
	cfg  Config
	mu   sync.Mutex
}

// New creates a Limiter whose state file lives under dataDir.
func New(dataDir string, cfg Config) *Limiter {
	return &Limiter{
		path: filepath.Join(dataDir, "ratelimit.json"),
		cfg:  cfg,
	}
}

type state struct {
	// SentAt holds unix-millisecond timestamps of past sends, pruned to
	// the largest configured window on every load.
	SentAt []int64 `json:"sentAt"`
}

// window pairs a limit with its duration for the check loop below.
type window struct {
	name  string
	limit int
	dur   time.Duration
}

func (c Config) windows() []window {
	var ws []window
	if c.PerMinute > 0 {
		ws = append(ws, window{"minute", c.PerMinute, time.Minute})
	}
	if c.PerHour > 0 {
		ws = append(ws, window{"hour", c.PerHour, time.Hour})
	}
	if c.PerDay > 0 {
		ws = append(ws, window{"day", c.PerDay, 24 * time.Hour})
	}
	return ws
}

// Allow reports whether a send is currently permitted under every
// configured window. If not, it returns the wait time until the earliest
// blocking window frees up a slot, and which window blocked it.
func (l *Limiter) Allow() (ok bool, retryAfter time.Duration, blockedWindow string, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	st, err := l.load()
	if err != nil {
		return false, 0, "", err
	}

	now := time.Now()
	var maxRetry time.Duration
	var blocker string

	for _, w := range l.cfg.windows() {
		cutoff := now.Add(-w.dur).UnixMilli()
		count := 0
		oldestInWindow := int64(0)
		for _, ts := range st.SentAt {
			if ts >= cutoff {
				count++
				if oldestInWindow == 0 || ts < oldestInWindow {
					oldestInWindow = ts
				}
			}
		}
		if count >= w.limit {
			retry := time.Until(time.UnixMilli(oldestInWindow).Add(w.dur))
			if retry > maxRetry {
				maxRetry = retry
				blocker = w.name
			}
		}
	}

	if blocker != "" {
		return false, maxRetry, blocker, nil
	}
	return true, 0, "", nil
}

// Record logs a successful send. Call this after the message actually goes
// out, not before — a failed send shouldn't count against the limit.
func (l *Limiter) Record() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	st, err := l.load()
	if err != nil {
		return err
	}
	st.SentAt = append(st.SentAt, time.Now().UnixMilli())
	return l.save(st)
}

func (l *Limiter) load() (state, error) {
	data, err := os.ReadFile(l.path)
	if os.IsNotExist(err) {
		return state{}, nil
	}
	if err != nil {
		return state{}, fmt.Errorf("reading rate limit state: %w", err)
	}

	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return state{}, fmt.Errorf("parsing rate limit state: %w", err)
	}

	// Prune anything older than the largest configured window so the file
	// doesn't grow forever.
	maxWindow := 24 * time.Hour
	for _, w := range l.cfg.windows() {
		if w.dur > maxWindow {
			maxWindow = w.dur
		}
	}
	cutoff := time.Now().Add(-maxWindow).UnixMilli()
	pruned := st.SentAt[:0]
	for _, ts := range st.SentAt {
		if ts >= cutoff {
			pruned = append(pruned, ts)
		}
	}
	st.SentAt = pruned

	return st, nil
}

func (l *Limiter) save(st state) error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}
	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("encoding rate limit state: %w", err)
	}
	if err := os.WriteFile(l.path, data, 0o600); err != nil {
		return fmt.Errorf("writing rate limit state: %w", err)
	}
	return nil
}
