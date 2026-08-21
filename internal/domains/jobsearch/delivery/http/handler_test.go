package http

import (
	"ai-assistant/internal/domains/jobsearch/domain"
	"bytes"
	"context"
	"github.com/gofiber/fiber/v2"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeDeliverer struct{ ch chan domain.Criteria }

func (f fakeDeliverer) SearchAndDeliver(_ context.Context, c domain.Criteria) error {
	f.ch <- c
	return nil
}
func TestHandlerAcceptsAndStartsIndependentWork(t *testing.T) {
	app := fiber.New()
	f := fakeDeliverer{make(chan domain.Criteria, 1)}
	NewHandler(f).Register(app)
	req := httptest.NewRequest("POST", "/loker", bytes.NewBufferString(`{"query":"Engineer | go | 1-3 | Jakarta | halal"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 202 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	select {
	case c := <-f.ch:
		if !c.Halal || !c.Interactive {
			t.Fatalf("criteria=%#v", c)
		}
	case <-time.After(time.Second):
		t.Fatal("background delivery not started")
	}
}
