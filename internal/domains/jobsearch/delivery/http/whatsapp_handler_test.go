package http

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

type fakeMessageSender struct {
	message string
	err     error
}

func (s *fakeMessageSender) Send(_ context.Context, message string) error {
	s.message = message
	return s.err
}

func TestWhatsAppHandlerSendsMessage(t *testing.T) {
	app := fiber.New()
	sender := &fakeMessageSender{}
	NewWhatsAppHandler(sender).Register(app)
	request := httptest.NewRequest("POST", "/internal/whatsapp/send", bytes.NewBufferString(`{"message":"jobs"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusNoContent || sender.message != "jobs" {
		t.Fatalf("status=%d message=%q", response.StatusCode, sender.message)
	}
}

func TestWhatsAppHandlerReportsDisabledAndSendFailure(t *testing.T) {
	app := fiber.New()
	NewWhatsAppHandler(nil).Register(app)
	request := httptest.NewRequest("POST", "/internal/whatsapp/send", bytes.NewBufferString(`{"message":"jobs"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ := app.Test(request)
	if response.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("disabled status=%d", response.StatusCode)
	}

	app = fiber.New()
	NewWhatsAppHandler(&fakeMessageSender{err: errors.New("offline")}).Register(app)
	request = httptest.NewRequest("POST", "/internal/whatsapp/send", bytes.NewBufferString(`{"message":"jobs"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = app.Test(request)
	if response.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("failure status=%d", response.StatusCode)
	}
}
