package whatsapp

import (
	"context"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"go.mau.fi/whatsmeow/types"
)

type Client struct {
	wac *whatsmeow.Client
}

func NewClient() (*Client, error) {
	ctx := context.Background()
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	container, err := sqlstore.New(ctx, "sqlite3", "file:stadium_sentinel.db?_foreign_keys=on", dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get device store: %w", err)
	}

	clientLog := waLog.Stdout("Client", "DEBUG", true)
	wac := whatsmeow.NewClient(deviceStore, clientLog)

	return &Client{wac: wac}, nil
}

// IsLoggedIn checks if the client already has a session token
func (c *Client) IsLoggedIn() bool {
	return c.wac.Store.ID != nil
}

// Connect starts the client.
func (c *Client) Connect() error {
	return c.wac.Connect()
}

// StartLoginFlow handles the new device pairing logic. 
// It calls qrCallback with the raw QR string. It blocks until paired or error.
func (c *Client) StartLoginFlow(ctx context.Context, qrCallback func(string)) error {
	qrChan, _ := c.wac.GetQRChannel(ctx)
	if err := c.wac.Connect(); err != nil {
		return err
	}
	
	pairingDetected := false
	for evt := range qrChan {
		if evt.Event == "code" {
			qrCallback(evt.Code)
		} else if evt.Event == "success" || evt.Event == "timeout" {
			// Pairing terminal state reached
			if evt.Event == "success" {
				pairingDetected = true
			}
		}
	}
	
	if !pairingDetected {
		return fmt.Errorf("pairing failed or timed out")
	}
	return nil
}

func (c *Client) SendAlert(phoneNumber string, message string) error {
	jid := types.NewJID(phoneNumber, types.DefaultUserServer)
	
	msg := &waProto.Message{
		Conversation: proto.String("🚨 *STADIUM SENTINEL ALERT* 🚨\n\n" + message),
	}

	_, err := c.wac.SendMessage(context.Background(), jid, msg)
	return err
}

func (c *Client) Disconnect() {
	if c.wac != nil {
		c.wac.Disconnect()
	}
}
