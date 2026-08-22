package application

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type deliveryFakeMessenger struct {
	mutex   sync.Mutex
	called  bool
	message string
	err     error
}

func (m *deliveryFakeMessenger) Send(_ context.Context, message string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.called = true
	m.message = message
	return m.err
}

func TestMultiMessengerAttemptsEveryDelivery(t *testing.T) {
	telegram := &deliveryFakeMessenger{}
	whatsapp := &deliveryFakeMessenger{err: errors.New("offline")}
	messenger := NewMultiMessenger([]Delivery{{Name: "telegram", Messenger: telegram}, {Name: "whatsapp", Messenger: whatsapp}}, nil)
	err := messenger.Send(t.Context(), "jobs")
	if err == nil {
		t.Fatal("expected aggregate delivery error")
	}
	if !telegram.called || telegram.message != "jobs" || !whatsapp.called {
		t.Fatalf("telegram=%#v whatsapp=%#v", telegram, whatsapp)
	}
}
