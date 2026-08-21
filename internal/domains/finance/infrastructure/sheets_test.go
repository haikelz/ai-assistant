package infrastructure

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"ai-assistant/internal/domains/finance/domain"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func TestGoogleSheetsSyncerInsertsRecordBeforeTotal(t *testing.T) {
	var insertedRow, updatedRow bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spreadsheets/spreadsheet-id"):
			_ = json.NewEncoder(w).Encode(map[string]any{"sheets": []any{map[string]any{"properties": map[string]any{"sheetId": 7, "title": "Juli"}}}})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/values/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"values": []any{[]any{"No", "Tanggal", "Nama Pengeluaran", "Jumlah", "Kategori", "Harga"}, []any{"1", "22/07/2026", "Makan pagi", "1", "Makan/Minum", "10000"}, []any{"Total"}}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			body, _ := io.ReadAll(r.Body)
			insertedRow = strings.Contains(string(body), `"insertDimension"`) && strings.Contains(string(body), `"startIndex":2`)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/values/"):
			var payload sheets.ValueRange
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			updatedRow = reflect.DeepEqual(payload.Values, [][]any{{float64(2), "23/07/2026", "Makan siang", float64(1), "Makan/Minum", float64(15000)}})
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer api.Close()
	service, err := sheets.NewService(context.Background(), option.WithHTTPClient(api.Client()))
	if err != nil {
		t.Fatal(err)
	}
	service.BasePath = api.URL + "/"
	location, err := time.LoadLocation(spreadsheetTimezone)
	if err != nil {
		t.Fatal(err)
	}
	syncer := &GoogleSheetsSyncer{service: service, spreadsheetID: "spreadsheet-id", location: location}
	status, err := syncer.Sync(t.Context(), domain.Record{Amount: 15000, Category: "Makan/Minum", Description: "Makan siang", CreatedAt: time.Date(2026, time.July, 23, 10, 30, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if status != domain.SyncSynced || !insertedRow || !updatedRow {
		t.Fatalf("status=%q inserted=%t updated=%t", status, insertedRow, updatedRow)
	}
}
