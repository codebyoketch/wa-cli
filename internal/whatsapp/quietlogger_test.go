package whatsapp

import (
	"testing"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// spyLogger records every call made to it, for asserting what a
// quietLogger did or didn't pass through.
type spyLogger struct {
	warnfCalls []string
}

func (s *spyLogger) Warnf(msg string, args ...interface{}) {
	s.warnfCalls = append(s.warnfCalls, msg)
}
func (s *spyLogger) Errorf(msg string, args ...interface{}) {}
func (s *spyLogger) Infof(msg string, args ...interface{})  {}
func (s *spyLogger) Debugf(msg string, args ...interface{}) {}
func (s *spyLogger) Sub(module string) waLog.Logger         { return s }

func TestQuietLogger_SuppressesCloseHandshakeWarning(t *testing.T) {
	spy := &spyLogger{}
	q := newQuietLogger(spy)

	q.Warnf("Error sending close to websocket: %v", "failed to close WebSocket: failed to read frame header: EOF")

	if len(spy.warnfCalls) != 0 {
		t.Errorf("expected the close-handshake warning to be suppressed, but it was logged: %v", spy.warnfCalls)
	}
}

func TestQuietLogger_PassesThroughOtherWarnings(t *testing.T) {
	spy := &spyLogger{}
	q := newQuietLogger(spy)

	q.Warnf("some other real problem: %v", "connection reset")

	if len(spy.warnfCalls) != 1 {
		t.Fatalf("expected 1 warning to pass through, got %d: %v", len(spy.warnfCalls), spy.warnfCalls)
	}
}

func TestQuietLogger_SubStillFilters(t *testing.T) {
	spy := &spyLogger{}
	q := newQuietLogger(spy)
	sub := q.Sub("Socket")

	sub.Warnf("Error sending close to websocket: %v", "failed to close WebSocket: EOF")

	if len(spy.warnfCalls) != 0 {
		t.Errorf("expected sub-logger to still suppress the warning, but it was logged: %v", spy.warnfCalls)
	}
}
