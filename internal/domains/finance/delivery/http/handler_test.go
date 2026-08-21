package http

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"ai-assistant/internal/domains/finance/application"
	"ai-assistant/internal/domains/finance/domain"
	"ai-assistant/internal/domains/finance/infrastructure"
	"github.com/gofiber/fiber/v2"
	_ "modernc.org/sqlite"
)

func TestRecordsAndTotalsContract(t *testing.T) {
	db, err := sql.Open("sqlite", t.TempDir()+"/finance.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := infrastructure.InitializeDatabase(db); err != nil {
		t.Fatal(err)
	}
	service := application.NewService(infrastructure.NewSQLiteRepository(db), infrastructure.DisabledSyncer{})
	app := fiber.New()
	NewHandler(service).Register(app)
	for _, body := range []string{
		`{"phone":"123","type":"modal","amount":5000000,"category":"Modal","description":"Gaji"}`,
		`{"phone":"123","type":"income","amount":250000,"category":"Pendapatan","description":"Bonus"}`,
		`{"phone":"123","type":"expense","amount":125000,"category":"Makan/Minum","description":"Makan"}`,
	} {
		req := httptest.NewRequest("POST", "/records", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		response, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 201 {
			t.Fatalf("create status=%d", response.StatusCode)
		}
		var created struct {
			SpreadsheetSyncStatus domain.SyncStatus `json:"spreadsheet_sync_status"`
		}
		if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
			t.Fatal(err)
		}
		if created.SpreadsheetSyncStatus != domain.SyncDisabled {
			t.Fatalf("sync=%q", created.SpreadsheetSyncStatus)
		}
	}
	response, err := app.Test(httptest.NewRequest("GET", "/totals?phone=123", nil))
	if err != nil {
		t.Fatal(err)
	}
	var totals domain.Totals
	if err := json.NewDecoder(response.Body).Decode(&totals); err != nil {
		t.Fatal(err)
	}
	if totals != (domain.Totals{Modal: 5000000, Income: 250000, Expense: 125000, Money: 5125000}) {
		t.Fatalf("totals=%#v", totals)
	}
}
