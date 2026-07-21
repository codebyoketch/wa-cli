package whatsapp

import (
	"fmt"
	"strings"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// quietLogger wraps a waLog.Logger and drops one specific, known-benign
// warning: the close-handshake race where WA.Disconnect() is called
// (as every one-shot command's `defer client.Disconnect()` does,
// immediately after the command's actual work finishes) and the server
// has already torn down its end of the socket, so whatsmeow's graceful
// close write/read fails with EOF and logs it at WARN:
//
//	Error sending close to websocket: failed to close WebSocket: failed
//	to read frame header: EOF
//
// The send/receive that already happened is unaffected either way —
// this is purely about the close handshake afterward — but because
// nearly every wa-cli command connects, does one thing, and disconnects
// right away, it was printing on almost every single invocation and
// drowning out real diagnostics for something that isn't one. Anything
// else at WARN or above, including genuine connection problems during
// `wa watch`, still prints normally; only this one message is filtered.
type quietLogger struct {
	waLog.Logger
}

// suppressedWarnings lists substrings matched against a Warnf call's
// fully-interpolated message. A call is dropped if any of these appear
// anywhere in it.
var suppressedWarnings = []string{
	"failed to close WebSocket",
}

func newQuietLogger(base waLog.Logger) waLog.Logger {
	return quietLogger{Logger: base}
}

// Warnf overrides the embedded Logger's Warnf to drop suppressedWarnings
// matches; everything else passes through to the real logger unchanged.
func (q quietLogger) Warnf(msg string, args ...interface{}) {
	full := fmt.Sprintf(msg, args...)
	for _, s := range suppressedWarnings {
		if strings.Contains(full, s) {
			return
		}
	}
	q.Logger.Warnf(msg, args...)
}

// Sub overrides the embedded Logger's Sub so the filter also applies to
// every sub-logger whatsmeow creates (e.g. "Client/Socket"), not just
// calls made directly on the top-level "Client" logger.
func (q quietLogger) Sub(module string) waLog.Logger {
	return quietLogger{Logger: q.Logger.Sub(module)}
}
