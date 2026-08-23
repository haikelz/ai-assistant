package app

import (
	"context"
	"testing"
)

type interactiveMessenger struct{}

func (interactiveMessenger) Send(context.Context, string) error { return nil }

func TestInteractiveJobDeliveryIsTelegramOnly(t *testing.T) {
	deliveries := interactiveJobDeliveries(interactiveMessenger{})
	if len(deliveries) != 1 || deliveries[0].Name != "telegram" {
		t.Fatalf("interactive deliveries=%#v", deliveries)
	}
}
