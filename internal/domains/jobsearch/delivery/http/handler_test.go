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

type fakeDeliverer struct {
	acknowledgementStarted chan struct{}
	releaseAcknowledgement chan struct{}
	criteria               chan domain.Criteria
}

func (f fakeDeliverer) AcknowledgeSearch(context.Context) error {
	close(f.acknowledgementStarted)
	<-f.releaseAcknowledgement
	return nil
}

func (f fakeDeliverer) SearchAndDeliver(_ context.Context, c domain.Criteria) error {
	f.criteria <- c
	return nil
}
func TestHandlerAcceptsAndStartsIndependentWork(t *testing.T) {
	app := fiber.New()
	f := fakeDeliverer{acknowledgementStarted: make(chan struct{}), releaseAcknowledgement: make(chan struct{}), criteria: make(chan domain.Criteria, 1)}
	NewHandler(f).Register(app)
	req := httptest.NewRequest("POST", "/loker", bytes.NewBufferString(`{"query":"Engineer | go | 1-3 | Jakarta | halal"}`))
	req.Header.Set("Content-Type", "application/json")
	response := make(chan int, 1)
	go func() {
		resp, err := app.Test(req)
		if err != nil {
			response <- 0
			return
		}
		response <- resp.StatusCode
	}()
	select {
	case <-f.acknowledgementStarted:
	case <-time.After(time.Second):
		t.Fatal("acknowledgement not started")
	}
	select {
	case criteria := <-f.criteria:
		t.Fatalf("search started before acknowledgement completed: %#v", criteria)
	case <-time.After(50 * time.Millisecond):
	}
	close(f.releaseAcknowledgement)
	if status := <-response; status != 202 {
		t.Fatalf("status=%d", status)
	}
	select {
	case c := <-f.criteria:
		if !c.Halal || !c.Interactive {
			t.Fatalf("criteria=%#v", c)
		}
	case <-time.After(time.Second):
		t.Fatal("background delivery not started")
	}
}
