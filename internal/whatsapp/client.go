// Package whatsapp wraps whatsmeow to provide wa-cli's login, logout, and
// status operations.
package whatsapp

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/codebyoketch/wa-cli/internal/qr"

	waerrors "github.com/codebyoketch/wa-cli/internal/errors"
)

// Client wraps a *whatsmeow.Client with wa-cli-specific login/logout flow.
type Client struct {
	WA  *whatsmeow.Client
	log waLog.Logger
}

// New builds a Client using the first (or a fresh, unpaired) device from container.
func New(ctx context.Context, container *sqlstore.Container, log waLog.Logger) (*Client, error) {
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, waerrors.Wrap(err, "loading device store")
	}

	clientLog := waLog.Stdout("Client", "WARN", true)
	waClient := whatsmeow.NewClient(device, clientLog)

	return &Client{WA: waClient, log: log}, nil
}

// IsLoggedIn reports whether this device already has a paired session.
func (c *Client) IsLoggedIn() bool {
	return c.WA.Store.ID != nil
}

// Login connects the client, printing an ASCII QR code to scan if this
// device isn't already paired. It blocks until login succeeds or the QR
// code expires.
func (c *Client) Login(ctx context.Context) error {
	if c.IsLoggedIn() {
		return c.WA.Connect()
	}

	qrChan, err := c.WA.GetQRChannel(ctx)
	if err != nil {
		return waerrors.Wrap(err, "getting QR channel")
	}

	if err := c.WA.Connect(); err != nil {
		return waerrors.Wrap(err, "connecting to WhatsApp")
	}

	for evt := range qrChan {
		switch evt.Event {
		case "code":
			fmt.Println("Scan this QR code in WhatsApp: Settings > Linked Devices > Link a Device")
			qr.Print(evt.Code)
		case "success":
			fmt.Println("Login successful.")
			return nil
		case "timeout":
			return waerrors.New("QR code expired, run 'wa login' again")
		default:
			c.log.Infof("login event: %s", evt.Event)
		}
	}

	return nil
}

// Logout logs the device out of WhatsApp and clears local session data.
// The client must already be connected.
func (c *Client) Logout(ctx context.Context) error {
	if !c.IsLoggedIn() {
		return waerrors.ErrNotLoggedIn
	}
	if err := c.WA.Logout(ctx); err != nil {
		return waerrors.Wrap(err, "logging out")
	}
	return nil
}

// Disconnect closes the WhatsApp connection without logging out.
func (c *Client) Disconnect() {
	c.WA.Disconnect()
}
