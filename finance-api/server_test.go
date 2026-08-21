package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFinanceAPIRecordsAndTotals(t *testing.T) {
	s := testServer(t, "")
	api := httptest.NewServer(s.routes())
	defer api.Close()

	for _, body := range []string{
		`{"phone":"123","type":"modal","amount":5000000,"category":"Modal","description":"Gaji"}`,
		`{"phone":"123","type":"income","amount":250000,"category":"Pendapatan","description":"Bonus"}`,
		`{"phone":"123","type":"expense","amount":125000,"category":"Makan/Minum","description":"Makan"}`,
	} {
		response, err := http.Post(api.URL+"/records", "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("create record status = %d, want %d", response.StatusCode, http.StatusCreated)
		}
	}

	response, err := http.Get(api.URL + "/totals?phone=123")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	got, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(got) != "{\"modal\":5000000,\"income\":250000,\"expense\":125000,\"money\":5125000}\n" {
		t.Fatalf("totals = status %d, body %s", response.StatusCode, got)
	}
}

func TestFinanceAPIProxiesResponsesToSumopod(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sumopod-key" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{"model":"deepseek-v4-flash","temperature":0.7}` {
			t.Errorf("DeepSeek body changed: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_123"}`))
	}))
	defer upstream.Close()

	s := testServer(t, upstream.URL+"/v1/responses")
	request := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"deepseek-v4-flash","temperature":0.7}`))
	request.Header.Set("Authorization", "Bearer sumopod-key")
	response := httptest.NewRecorder()
	s.routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != `{"id":"resp_123"}` {
		t.Fatalf("proxy = status %d, body %s", response.Code, response.Body.String())
	}
}

func TestFinanceAPIOmitsTemperatureForSumopodGPTResponses(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if _, exists := request["temperature"]; exists {
			t.Error("GPT Responses request must not include temperature")
		}
		var model string
		if err := json.Unmarshal(request["model"], &model); err != nil {
			t.Fatal(err)
		}
		if model != "openai/gpt-5" {
			t.Errorf("model = %q", model)
		}
		if _, exists := request["max_output_tokens"]; !exists {
			t.Error("unrelated Responses fields must be preserved")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_gpt"}`))
	}))
	defer upstream.Close()

	s := testServer(t, upstream.URL+"/v1/responses")
	request := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"openai/gpt-5","temperature":0.7,"max_output_tokens":2048}`))
	request.Header.Set("Authorization", "Bearer sumopod-key")
	response := httptest.NewRecorder()
	s.routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != `{"id":"resp_gpt"}` {
		t.Fatalf("proxy = status %d, body %s", response.Code, response.Body.String())
	}
}

func TestFinanceAPIConvertsSumopodGPTEventStreamToCompletedResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_gpt\",\"status\":\"in_progress\"}}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_gpt\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"Hello\"}]}]}}\n\n")
	}))
	defer upstream.Close()

	s := testServer(t, upstream.URL+"/v1/responses")
	request := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.6-luna","temperature":0.7}`))
	response := httptest.NewRecorder()
	s.routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("proxy status = %d, body %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %q", response.Header().Get("Content-Type"))
	}
	var completed struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&completed); err != nil {
		t.Fatal(err)
	}
	if completed.ID != "resp_gpt" || completed.Status != "completed" {
		t.Fatalf("completed response = %#v", completed)
	}
}

func testServer(t *testing.T, sumopodURL string) *server {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := initializeDatabase(db); err != nil {
		t.Fatal(err)
	}
	if sumopodURL == "" {
		sumopodURL = "http://127.0.0.1/unused"
	}
	return newServer(db, http.DefaultClient, sumopodURL)
}
