package infrastructure

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestConsumeWhatsAppQRReportsLifecycleWithoutRawErrorDetailsElsewhere(t *testing.T) {
	events := make(chan whatsmeow.QRChannelItem, 3)
	events <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "pairing-code"}
	events <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventError, Error: errors.New("failed")}
	events <- whatsmeow.QRChannelSuccess
	close(events)
	var output bytes.Buffer
	consumeWhatsAppQR(events, &output)
	for _, expected := range []string{"scan QR", "pairing error: failed", "paired successfully"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output missing %q", expected)
		}
	}
}

func TestOpenWhatsAppStorePersistsDeviceWithoutNetwork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "whatsapp.db")
	store, err := openWhatsAppStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	device, err := store.GetFirstDevice(t.Context())
	if err != nil || device == nil {
		t.Fatalf("device=%v err=%v", device, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = openWhatsAppStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if device, err = store.GetFirstDevice(t.Context()); err != nil || device == nil {
		t.Fatalf("reopened device=%v err=%v", device, err)
	}
}
