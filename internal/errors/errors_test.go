package errors

import (
	"strings"
	"testing"
)

func TestWrap_NilReturnsNil(t *testing.T) {
	if err := Wrap(nil, "context"); err != nil {
		t.Fatalf("Wrap(nil, ...) = %v, want nil", err)
	}
}

func TestWrap_AddsContextAndPreservesIs(t *testing.T) {
	wrapped := Wrap(ErrNotLoggedIn, "checking session")

	if !strings.Contains(wrapped.Error(), "checking session") {
		t.Errorf("wrapped error %q missing context prefix", wrapped.Error())
	}
	if !strings.Contains(wrapped.Error(), ErrNotLoggedIn.Error()) {
		t.Errorf("wrapped error %q missing original message", wrapped.Error())
	}
	if !Is(wrapped, ErrNotLoggedIn) {
		t.Error("Is(wrapped, ErrNotLoggedIn) = false, want true — Wrap must preserve the sentinel for errors.Is")
	}
}

func TestWrapf_NilReturnsNil(t *testing.T) {
	if err := Wrapf(nil, "context %d", 1); err != nil {
		t.Fatalf("Wrapf(nil, ...) = %v, want nil", err)
	}
}

func TestWrapf_FormatsAndPreservesIs(t *testing.T) {
	wrapped := Wrapf(ErrConfigNotFound, "loading %s", "config.json")

	if !strings.Contains(wrapped.Error(), "loading config.json") {
		t.Errorf("wrapped error %q missing formatted context", wrapped.Error())
	}
	if !Is(wrapped, ErrConfigNotFound) {
		t.Error("Is(wrapped, ErrConfigNotFound) = false, want true")
	}
}

func TestNew_IsDistinctSentinel(t *testing.T) {
	a := New("boom")
	b := New("boom")
	// Same message, but New (== stdlib errors.New) must return distinct
	// identities — errors.Is compares by identity, not message text.
	if Is(a, b) {
		t.Error("two separately-constructed errors with the same message compared equal via Is; sentinel identity is broken")
	}
}
