package infrastructure

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalWhatsAppPostsMessage(t *testing.T) {
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var body struct {
			Message string `json:"message"`
		}
		if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&body) != nil {
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		received = body.Message
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if err := NewLocalWhatsApp(server.Client(), server.URL).Send(t.Context(), "jobs"); err != nil {
		t.Fatal(err)
	}
	if received != "jobs" {
		t.Fatalf("received=%q", received)
	}
}

func TestLocalWhatsAppReturnsEndpointFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, "offline", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	if err := NewLocalWhatsApp(server.Client(), server.URL).Send(t.Context(), "jobs"); err == nil {
		t.Fatal("expected endpoint failure")
	}
}
