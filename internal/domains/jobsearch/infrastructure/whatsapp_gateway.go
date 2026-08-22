package infrastructure

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

type WhatsAppGateway struct {
	client *whatsmeow.Client
	store  *sqlstore.Container
}

func NewWhatsAppGateway(ctx context.Context, sessionPath string, output io.Writer) (*WhatsAppGateway, error) {
	if output == nil {
		output = os.Stdout
	}
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o700); err != nil {
		return nil, fmt.Errorf("create WhatsApp session directory: %w", err)
	}
	store, err := openWhatsAppStore(ctx, sessionPath)
	if err != nil {
		return nil, fmt.Errorf("open WhatsApp session: %w", err)
	}
	device, err := store.GetFirstDevice(ctx)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("load WhatsApp device: %w", err)
	}
	client := whatsmeow.NewClient(device, waLog.Stdout("whatsapp", "INFO", false))
	gateway := &WhatsAppGateway{client: client, store: store}
	if client.Store.ID == nil {
		qrEvents, qrErr := client.GetQRChannel(ctx)
		if qrErr != nil {
			_ = store.Close()
			return nil, fmt.Errorf("start WhatsApp QR pairing: %w", qrErr)
		}
		go consumeWhatsAppQR(qrEvents, output)
	} else {
		log.Printf("whatsapp: stored session found; reconnecting")
	}
	if err := client.ConnectContext(ctx); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("connect WhatsApp: %w", err)
	}
	return gateway, nil
}

func openWhatsAppStore(ctx context.Context, sessionPath string) (*sqlstore.Container, error) {
	return sqlstore.New(ctx, "sqlite", "file:"+sessionPath+"?_pragma=foreign_keys(1)", waLog.Stdout("whatsapp-db", "WARN", false))
}

func consumeWhatsAppQR(events <-chan whatsmeow.QRChannelItem, output io.Writer) {
	for event := range events {
		switch event.Event {
		case whatsmeow.QRChannelEventCode:
			fmt.Fprintln(output, "whatsapp: scan QR berikut melalui WhatsApp > Perangkat tertaut > Tautkan perangkat")
			fmt.Fprintln(output, "whatsapp: QR bersifat rahasia dan akan kedaluwarsa; jangan bagikan log ini")
			qrterminal.GenerateHalfBlock(event.Code, qrterminal.M, output)
		case whatsmeow.QRChannelSuccess.Event:
			fmt.Fprintln(output, "whatsapp: paired successfully")
		case whatsmeow.QRChannelTimeout.Event:
			fmt.Fprintln(output, "whatsapp: pairing timed out; restart pod untuk menampilkan QR baru")
		case whatsmeow.QRChannelEventError:
			fmt.Fprintf(output, "whatsapp: pairing error: %v\n", event.Error)
		default:
			fmt.Fprintf(output, "whatsapp: pairing stopped (%s); restart pod untuk mencoba lagi\n", event.Event)
		}
	}
}

func (g *WhatsAppGateway) Ready(timeout time.Duration) bool {
	if g == nil || g.client == nil || g.client.Store.ID == nil {
		return false
	}
	if g.client.IsLoggedIn() {
		return true
	}
	return g.client.WaitForConnection(timeout) && g.client.IsLoggedIn()
}

func (g *WhatsAppGateway) SendText(ctx context.Context, recipient, text string) error {
	_, err := g.client.SendMessage(ctx, types.NewJID(recipient, types.DefaultUserServer), &waE2E.Message{Conversation: proto.String(text)})
	return err
}

func (g *WhatsAppGateway) Close() error {
	if g == nil {
		return nil
	}
	if g.client != nil {
		g.client.Disconnect()
	}
	if g.store != nil {
		return g.store.Close()
	}
	return nil
}
