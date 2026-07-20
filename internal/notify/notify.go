// Package notify sends OS desktop notifications for incoming WhatsApp
// messages. It's a thin wrapper around beeep so the rest of the codebase
// doesn't depend on a third-party notification library directly.
package notify

import "github.com/gen2brain/beeep"

// Send shows a desktop notification with the given title and body.
// Desktop notification support varies a lot by OS/environment (e.g. a
// minimal Linux setup without notify-send installed) — callers should
// log a failure here, not treat it as fatal, since a missed popup
// shouldn't take down message receiving.
func Send(title, body string) error {
	return beeep.Notify(title, body, "")
}
