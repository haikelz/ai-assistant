package infrastructure

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPrepareResponsesBodyPreservesDeepSeekBytes(t *testing.T) {
	body := []byte(`{"model":"deepseek-v4-flash", "temperature":0.7}`)
	got, isGPT, err := PrepareResponsesBody(body)
	if err != nil || isGPT || !bytes.Equal(got, body) {
		t.Fatalf("got=%s gpt=%t err=%v", got, isGPT, err)
	}
}

func TestSumopodGPTRemovesTemperatureAndConvertsSSE(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if _, ok := request["temperature"]; ok {
			t.Error("temperature was forwarded")
		}
		if _, ok := request["max_output_tokens"]; !ok {
			t.Error("unrelated field was removed")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.created\"}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\"}}\n\n")
	}))
	defer upstream.Close()
	proxy := NewSumopodProxy(upstream.Client(), upstream.URL)
	response, err := proxy.Forward(t.Context(), "Bearer key", []byte(`{"model":"gpt-5.6-luna","temperature":1,"max_output_tokens":20}`))
	if err != nil {
		t.Fatal(err)
	}
	if response.ContentType != "application/json" || string(response.Body) != `{"id":"resp_1","status":"completed"}` {
		t.Fatalf("response=%#v", response)
	}
}
