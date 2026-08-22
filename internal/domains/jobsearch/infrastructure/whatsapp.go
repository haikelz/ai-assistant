package infrastructure

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const whatsAppMessageLimit = 4000

type WhatsAppTextSender interface {
	Ready(time.Duration) bool
	SendText(context.Context, string, string) error
}

type WhatsApp struct {
	sender    WhatsAppTextSender
	recipient string
}

func NewWhatsApp(sender WhatsAppTextSender, recipient string) (*WhatsApp, error) {
	normalized, err := NormalizeWhatsAppRecipient(recipient)
	if err != nil {
		return nil, err
	}
	if sender == nil {
		return nil, fmt.Errorf("WhatsApp sender is required")
	}
	return &WhatsApp{sender: sender, recipient: normalized}, nil
}

func NormalizeWhatsAppRecipient(recipient string) (string, error) {
	var digits strings.Builder
	for _, character := range strings.TrimSpace(recipient) {
		if character >= '0' && character <= '9' {
			digits.WriteRune(character)
			continue
		}
		if character != '+' && character != ' ' && character != '-' && character != '(' && character != ')' && character != '.' {
			return "", fmt.Errorf("invalid WhatsApp recipient character %q", character)
		}
	}
	normalized := strings.TrimLeft(digits.String(), "0")
	if strings.HasPrefix(digits.String(), "0") {
		normalized = "62" + strings.TrimLeft(digits.String(), "0")
	}
	if len(normalized) < 8 || len(normalized) > 15 {
		return "", fmt.Errorf("WhatsApp recipient must contain 8-15 international digits")
	}
	return normalized, nil
}

func (w *WhatsApp) Send(ctx context.Context, message string) error {
	if !w.sender.Ready(10 * time.Second) {
		return fmt.Errorf("WhatsApp is not paired or connected")
	}
	message = strings.ReplaceAll(message, "**", "*")
	for _, chunk := range splitWhatsAppMessage(message, whatsAppMessageLimit) {
		if err := w.sender.SendText(ctx, w.recipient, chunk); err != nil {
			return err
		}
	}
	return nil
}

func splitWhatsAppMessage(message string, maxRunes int) []string {
	if maxRunes <= 0 {
		return nil
	}
	runes := []rune(message)
	var chunks []string
	for len(runes) > maxRunes {
		cut := maxRunes
		for index := maxRunes; index > maxRunes/2; index-- {
			if runes[index-1] == '\n' {
				cut = index
				break
			}
		}
		chunks = append(chunks, string(runes[:cut]))
		runes = runes[cut:]
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}
