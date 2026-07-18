// Package errors provides wa-cli's shared error types and sentinels.
//
// It intentionally wraps the standard library's errors package so callers
// can `import "github.com/codebyoketch/wa-cli/internal/errors"` and still
// use errors.New, errors.Is, errors.As, errors.Wrap-style helpers below.
package errors

import (
	"errors"
	"fmt"
)

// Re-exported standard library helpers so callers only need one import.
var (
	New = errors.New
	Is  = errors.Is
	As  = errors.As
)

// Sentinel errors shared across commands.
var (
	// ErrNotLoggedIn is returned when a command that requires an active
	// WhatsApp session is run before `wa login`.
	ErrNotLoggedIn = errors.New("not logged in: run 'wa login' first")

	// ErrConfigNotFound is returned when no config file exists yet.
	ErrConfigNotFound = errors.New("config not found: run 'wa config init' first")

	// ErrNotImplemented marks commands that are scaffolded but not yet built.
	ErrNotImplemented = errors.New("not implemented yet")
)

// Wrap adds context to err while preserving it for errors.Is/As, e.g.:
//
//	if err != nil {
//	    return errors.Wrap(err, "loading config")
//	}
func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// Wrapf is Wrap with fmt-style formatting.
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(fmt.Sprintf(format, args...)+": %w", err)
}
