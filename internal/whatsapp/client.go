package whatsapp

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/codebyoketch/wa-cli/internal/qr"
	waerrors "github.com/codebyoketch/wa-cli/internal/errors"
)

type Client struct {
	WA  *whatsmeow.Client
	log waLog.Logger
}

func New(ctx context.Context, container *sqlstore.Container, log waLog.Logger) (*Client, error) {
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, waerrors.Wrap(err, "loading device store")
	}
	clientLog := waLog.Stdout("Client", "WARN", true)
	waClient := whatsmeow.NewClient(device, clientLog)
	return &Client{WA: waClient, log: log}, nil
}

func (c *Client) IsLoggedIn() bool {
	return c.WA.Store.ID != nil
}

// Login connects and, if unpaired, shows a QR code to scan. It stays
// connected until the post-pairing sync actually completes (or times out),
// so the phone has time to commit the linked device — disconnecting right
// after the QR "success" event is too early and the device never shows up
// in WhatsApp > Linked Devices.
func (c *Client) Login(ctx context.Context) error {
	// Fires once the full connection (including post-pairing sync) is ready.
	connected := make(chan struct{}, 1)
	c.WA.AddEventHandler(func(evt interface{}) {
		switch evt.(type) {
		case *events.Connected:
			select {
			case connected <- struct{}{}:
			default:
			}
		}
	})

	if c.IsLoggedIn() {
		if err := c.WA.Connect(); err != nil {
			return waerrors.Wrap(err, "connecting to WhatsApp")
		}
		<-connected
		return nil
	}

	qrChan, err := c.WA.GetQRChannel(ctx)
	if err != nil {
		return waerrors.Wrap(err, "getting QR channel")
	}

	if err := c.WA.Connect(); err != nil {
		return waerrors.Wrap(err, "connecting to WhatsApp")
	}

	paired := false
	for evt := range qrChan {
		switch evt.Event {
		case "code":
			fmt.Println("Scan this QR code in WhatsApp: Settings > Linked Devices > Link a Device")
			qr.Print(evt.Code)
		case "success":
			paired = true
			fmt.Println("QR scanned — finishing setup, keep this open...")
		case "timeout":
			return waerrors.New("QR code expired, run 'wa login' again")
		default:
			c.log.Infof("login event: %s", evt.Event)
		}
	}

	if !paired {
		return waerrors.New("login did not complete")
	}

	// Wait for the actual post-pairing sync to finish, capped so we don't
	// hang forever if something's wrong.
	select {
	case <-connected:
		fmt.Println("Login successful.")
	case <-time.After(20 * time.Second):
		fmt.Println("Paired, but sync is taking a while — check Linked Devices on your phone. If it's not there, run 'wa logout' then 'wa login' again.")
	}

	return nil
}

func (c *Client) Logout(ctx context.Context) error {
	if !c.IsLoggedIn() {
		return waerrors.ErrNotLoggedIn
	}
	if err := c.WA.Logout(ctx); err != nil {
		return waerrors.Wrap(err, "logging out")
	}
	return nil
}

func (c *Client) Disconnect() {
	c.WA.Disconnect()
}
