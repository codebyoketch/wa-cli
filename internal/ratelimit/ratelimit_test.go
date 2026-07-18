package ratelimit

import (
	"testing"
	"time"
)

func TestAllow_UnderLimit(t *testing.T) {
	dir := t.TempDir()
	l := New(dir, Config{PerMinute: 3})

	for i := 0; i < 3; i++ {
		ok, _, _, err := l.Allow()
		if err != nil {
			t.Fatalf("Allow: %v", err)
		}
		if !ok {
			t.Fatalf("send %d: expected allowed, got blocked", i)
		}
		if err := l.Record(); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
}

func TestAllow_BlocksOverLimit(t *testing.T) {
	dir := t.TempDir()
	l := New(dir, Config{PerMinute: 2})

	for i := 0; i < 2; i++ {
		if err := l.Record(); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	ok, retry, blocked, err := l.Allow()
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if ok {
		t.Fatal("expected blocked, got allowed")
	}
	if blocked != "minute" {
		t.Fatalf("expected blocked window 'minute', got %q", blocked)
	}
	if retry <= 0 || retry > time.Minute {
		t.Fatalf("expected retryAfter in (0, 1m], got %v", retry)
	}
}

func TestAllow_MultipleWindowsMostRestrictiveWins(t *testing.T) {
	dir := t.TempDir()
	// Hour limit is tighter than minute limit here.
	l := New(dir, Config{PerMinute: 100, PerHour: 1})

	if err := l.Record(); err != nil {
		t.Fatalf("Record: %v", err)
	}

	ok, _, blocked, err := l.Allow()
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if ok {
		t.Fatal("expected blocked by hour window, got allowed")
	}
	if blocked != "hour" {
		t.Fatalf("expected blocked window 'hour', got %q", blocked)
	}
}

func TestAllow_NoLimitsConfigured(t *testing.T) {
	dir := t.TempDir()
	l := New(dir, Config{})

	ok, _, _, err := l.Allow()
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !ok {
		t.Fatal("expected allowed when no limits configured")
	}
}

func TestPersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()

	l1 := New(dir, Config{PerMinute: 1})
	if err := l1.Record(); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// A fresh Limiter pointed at the same dir should see the prior send.
	l2 := New(dir, Config{PerMinute: 1})
	ok, _, _, err := l2.Allow()
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if ok {
		t.Fatal("expected blocked, state should have persisted to disk")
	}
}
