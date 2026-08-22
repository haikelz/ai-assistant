package infrastructure

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeWhatsAppTextSender struct {
	ready      bool
	recipients []string
	messages   []string
	err        error
}

func (s *fakeWhatsAppTextSender) Ready(time.Duration) bool { return s.ready }
func (s *fakeWhatsAppTextSender) SendText(_ context.Context, recipient, message string) error {
	s.recipients = append(s.recipients, recipient)
	s.messages = append(s.messages, message)
	return s.err
}

func TestNormalizeWhatsAppRecipient(t *testing.T) {
	tests := map[string]string{
		"0812-3456-7890":  "6281234567890",
		"+62 812 345 678": "62812345678",
		"14155552671":     "14155552671",
	}
	for input, expected := range tests {
		actual, err := NormalizeWhatsAppRecipient(input)
		if err != nil || actual != expected {
			t.Errorf("NormalizeWhatsAppRecipient(%q)=(%q, %v), expected %q", input, actual, err, expected)
		}
	}
	for _, input := range []string{"", "0812abc", "123"} {
		if _, err := NormalizeWhatsAppRecipient(input); err == nil {
			t.Errorf("expected %q to be rejected", input)
		}
	}
}

func TestWhatsAppFormatsAndSendsChunksInOrder(t *testing.T) {
	sender := &fakeWhatsAppTextSender{ready: true}
	messenger, err := NewWhatsApp(sender, "081234567890")
	if err != nil {
		t.Fatal(err)
	}
	message := "**Daftar Job**\n" + strings.Repeat("界", whatsAppMessageLimit+10)
	if err := messenger.Send(t.Context(), message); err != nil {
		t.Fatal(err)
	}
	if len(sender.messages) != 2 {
		t.Fatalf("messages=%d", len(sender.messages))
	}
	if strings.Join(sender.messages, "") != strings.ReplaceAll(message, "**", "*") {
		t.Fatal("chunks do not preserve formatted message")
	}
	for index, recipient := range sender.recipients {
		if recipient != "6281234567890" {
			t.Fatalf("recipient[%d]=%q", index, recipient)
		}
	}
}

func TestWhatsAppDoesNotSendWhenOfflineOrRetryFailures(t *testing.T) {
	offline := &fakeWhatsAppTextSender{}
	messenger, _ := NewWhatsApp(offline, "081234567890")
	if err := messenger.Send(t.Context(), "jobs"); err == nil || len(offline.messages) != 0 {
		t.Fatalf("offline send err=%v messages=%v", err, offline.messages)
	}

	failing := &fakeWhatsAppTextSender{ready: true, err: errors.New("unknown delivery state")}
	messenger, _ = NewWhatsApp(failing, "081234567890")
	if err := messenger.Send(t.Context(), "jobs"); err == nil || len(failing.messages) != 1 {
		t.Fatalf("failed send err=%v messages=%v", err, failing.messages)
	}
}
