package main

import (
	"bytes"
	"database/sql"
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_123"}`))
	}))
	defer upstream.Close()

	s := testServer(t, upstream.URL+"/v1/responses")
	request := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(`{"model":"deepseek-v4-pro"}`))
	request.Header.Set("Authorization", "Bearer sumopod-key")
	response := httptest.NewRecorder()
	s.routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != `{"id":"resp_123"}` {
		t.Fatalf("proxy = status %d, body %s", response.Code, response.Body.String())
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
