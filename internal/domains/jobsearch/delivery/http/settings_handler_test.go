package http

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ai-assistant/internal/domains/jobsearch/application"
	"ai-assistant/internal/domains/jobsearch/infrastructure"
	"github.com/gofiber/fiber/v2"
)

func TestSettingsHandlerUpdatesAndReturnsCurrentConfig(t *testing.T) {
	store := infrastructure.NewJSONAlertConfigStore(filepath.Join(t.TempDir(), "job-alert.json"))
	app := fiber.New()
	NewSettingsHandler(application.NewSettingsService(store)).Register(app)

	request := httptest.NewRequest("POST", "/job-alert", bytes.NewBufferString(`{"query":"Software Engineer | go, typescript | 1-3 | Jakarta"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("update status=%d", response.StatusCode)
	}
	var updated struct {
		Configured bool `json:"configured"`
		Config     struct {
			Criteria struct {
				Halal    bool     `json:"halal"`
				MaxYears int      `json:"max_years"`
				Skills   []string `json:"skills"`
			} `json:"criteria"`
		} `json:"config"`
	}
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if !updated.Configured || !updated.Config.Criteria.Halal || updated.Config.Criteria.MaxYears != 3 || len(updated.Config.Criteria.Skills) != 2 {
		t.Fatalf("updated=%#v", updated)
	}

	response, err = app.Test(httptest.NewRequest("GET", "/job-alert", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("current status=%d", response.StatusCode)
	}
}

func TestSettingsHandlerReportsUnconfigured(t *testing.T) {
	app := fiber.New()
	store := infrastructure.NewJSONAlertConfigStore(filepath.Join(t.TempDir(), "missing.json"))
	NewSettingsHandler(application.NewSettingsService(store)).Register(app)
	response, err := app.Test(httptest.NewRequest("GET", "/job-alert", nil))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Configured bool `json:"configured"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Configured {
		t.Fatal("missing config reported as configured")
	}
}

func TestSettingsHandlerRejectsMissingPosition(t *testing.T) {
	app := fiber.New()
	store := infrastructure.NewJSONAlertConfigStore(filepath.Join(t.TempDir(), "job-alert.json"))
	NewSettingsHandler(application.NewSettingsService(store)).Register(app)
	request := httptest.NewRequest("POST", "/job-alert", bytes.NewBufferString(`{"query":" | go | 1-3 | Jakarta"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status=%d", response.StatusCode)
	}
}
