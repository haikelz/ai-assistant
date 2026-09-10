package http

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http/httptest"
	"testing"

	"ai-assistant/internal/domains/jobsearch/domain"
	"github.com/gofiber/fiber/v2"
)

type fakeDeliverer struct {
	acknowledgementStarted chan struct{}
	releaseAcknowledgement chan struct{}
	criteria               chan domain.Criteria
	searchCancelled        chan struct{}
}

func (f fakeDeliverer) AcknowledgeSearch(ctx context.Context) error {
	if f.acknowledgementStarted != nil {
		close(f.acknowledgementStarted)
	}
	if f.releaseAcknowledgement != nil {
		select {
		case <-f.releaseAcknowledgement:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (f fakeDeliverer) SearchAndDeliver(ctx context.Context, c domain.Criteria) error {
	if f.criteria != nil {
		f.criteria <- c
	}
	if f.searchCancelled != nil {
		<-ctx.Done()
		close(f.searchCancelled)
		return ctx.Err()
	}
	return nil
}

func TestHandlerAcceptsAndStartsIndependentWork(t *testing.T) {
	app := fiber.New()
	f := fakeDeliverer{acknowledgementStarted: make(chan struct{}), releaseAcknowledgement: make(chan struct{}), criteria: make(chan domain.Criteria, 1)}
	handler, err := NewJobSearchHandler(t.Context(), f, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := handler.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown handler: %v", err)
		}
	})
	handler.Register(app)
	req := httptest.NewRequest("POST", "/loker", bytes.NewBufferString(`{"query":"Engineer | go | 1-3 | Jakarta | halal"}`))
	req.Header.Set("Content-Type", "application/json")
	response := make(chan int, 1)
	go func() {
		resp, err := app.Test(req)
		if err != nil {
			response <- 0
			return
		}
		if err := resp.Body.Close(); err != nil {
			response <- 0
			return
		}
		response <- resp.StatusCode
	}()
	<-f.acknowledgementStarted
	select {
	case criteria := <-f.criteria:
		t.Fatalf("search started before acknowledgement completed: %#v", criteria)
	default:
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
	}
}

func TestHandlerCancelsAcceptedSearchDuringShutdown(t *testing.T) {
	app := fiber.New()
	searchStarted := make(chan domain.Criteria, 1)
	searchCancelled := make(chan struct{})
	handler, err := NewJobSearchHandler(t.Context(), fakeDeliverer{
		criteria:        searchStarted,
		searchCancelled: searchCancelled,
	}, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	handler.Register(app)

	request := httptest.NewRequest("POST", "/loker", bytes.NewBufferString(`{"query":"Engineer"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusAccepted {
		t.Fatalf("status=%d", response.StatusCode)
	}
	<-searchStarted
	if err := handler.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	<-searchCancelled

	request = httptest.NewRequest("POST", "/loker", bytes.NewBufferString(`{"query":"Engineer"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err = app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status after shutdown=%d", response.StatusCode)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
}
