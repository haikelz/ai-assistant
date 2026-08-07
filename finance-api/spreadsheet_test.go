package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func TestCreateRecord_reportsSyncedSpreadsheetRecord(t *testing.T) {
	// Given
	syncer := &fakeRecordSyncer{status: spreadsheetSyncStatusSynced}
	s := testServer(t, "")
	s.recordSyncer = syncer
	request := httptest.NewRequest(http.MethodPost, "/records", bytes.NewBufferString(`{"phone":"123","type":"expense","amount":15000,"category":"Makan/Minum","description":"Makan siang"}`))
	response := httptest.NewRecorder()

	// When
	s.routes().ServeHTTP(response, request)

	// Then
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusCreated)
	}
	var got createRecordResponse
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.SpreadsheetSyncStatus != spreadsheetSyncStatusSynced {
		t.Fatalf("spreadsheet sync status = %q, want %q", got.SpreadsheetSyncStatus, spreadsheetSyncStatusSynced)
	}
	if !syncer.called || syncer.record.Description != "Makan siang" {
		t.Fatalf("syncer record = %#v, called = %t", syncer.record, syncer.called)
	}
}

func TestSpreadsheetRow_matchesWorkbookHeader(t *testing.T) {
	// Given
	headers := []string{"No", "Tanggal", "Nama Pengeluaran", "Jumlah", "Kategori", "Harga"}
	record := record{
		Description: "Makan siang",
		Category:    "Makan/Minum",
		Amount:      15000,
		CreatedAt:   time.Date(2026, time.July, 23, 10, 30, 0, 0, time.FixedZone("WIB", 7*60*60)),
	}

	// When
	got, err := spreadsheetRow(headers, 9, record)

	// Then
	if err != nil {
		t.Fatalf("spreadsheet row: %v", err)
	}
	want := []any{9, "23/07/2026", "Makan siang", 1, "Makan/Minum", int64(15000)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("row = %#v, want %#v", got, want)
	}
}

func TestGoogleSheetsSyncer_insertsRecordBeforeTotal(t *testing.T) {
	// Given
	var insertedRow, updatedRow bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spreadsheets/spreadsheet-id"):
			_ = json.NewEncoder(w).Encode(map[string]any{"sheets": []any{map[string]any{"properties": map[string]any{"sheetId": 7, "title": "Juli"}}}})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/values/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"values": []any{
				[]any{"No", "Tanggal", "Nama Pengeluaran", "Jumlah", "Kategori", "Harga"},
				[]any{"1", "22/07/2026", "Makan pagi", "1", "Makan/Minum", "10000"},
				[]any{"Total"},
			}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read batch update body: %v", err)
			}
			insertedRow = strings.Contains(string(body), `"insertDimension"`) && strings.Contains(string(body), `"startIndex":2`)
			_ = json.NewEncoder(w).Encode(map[string]any{})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/values/"):
			var payload sheets.ValueRange
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode update body: %v", err)
			}
			updatedRow = reflect.DeepEqual(payload.Values, [][]any{{float64(2), "23/07/2026", "Makan siang", float64(1), "Makan/Minum", float64(15000)}})
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer api.Close()
	service, err := sheets.NewService(context.Background(), option.WithHTTPClient(api.Client()))
	if err != nil {
		t.Fatalf("create Sheets service: %v", err)
	}
	service.BasePath = api.URL + "/"
	location, err := time.LoadLocation(spreadsheetTimezone)
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	syncer := &googleSheetsSyncer{service: service, spreadsheetID: "spreadsheet-id", location: location}

	// When
	status, err := syncer.Sync(context.Background(), record{
		Amount:      15000,
		Category:    "Makan/Minum",
		Description: "Makan siang",
		CreatedAt:   time.Date(2026, time.July, 23, 10, 30, 0, 0, time.UTC),
	})

	// Then
	if err != nil {
		t.Fatalf("sync spreadsheet: %v", err)
	}
	if status != spreadsheetSyncStatusSynced || !insertedRow || !updatedRow {
		t.Fatalf("status = %q, inserted row = %t, updated row = %t", status, insertedRow, updatedRow)
	}
}

type fakeRecordSyncer struct {
	status spreadsheetSyncStatus
	err    error
	record record
	called bool
}

func (f *fakeRecordSyncer) Sync(_ context.Context, input record) (spreadsheetSyncStatus, error) {
	f.record = input
	f.called = true
	return f.status, f.err
}
