// Package qr renders WhatsApp pairing codes as ASCII QR codes for the terminal.
package qr

import (
	"os"

	"github.com/mdp/qrterminal/v3"
)

// Print writes an ASCII QR code for code to stdout.
func Print(code string) {
	qrterminal.GenerateHalfBlock(code, qrterminal.L, os.Stdout)
}
