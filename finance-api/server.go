package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	defaultDatabasePath    = "/root/.picoclaw/finance.db"
	defaultListenAddress   = "127.0.0.1:8080"
	defaultSumopodURL      = "https://ai.sumopod.com/v1/responses"
	maxRequestBodyBytes    = 8 << 20
	responseRequestTimeout = 120 * time.Second
)

type server struct {
	db                  *sql.DB
	httpClient          *http.Client
	sumopodResponsesURL string
	recordSyncer        recordSyncer
}

func main() {
	databasePath := getenv("FINANCE_DB_PATH", defaultDatabasePath)
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		panic(fmt.Errorf("create finance database directory: %w", err))
	}

	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		panic(fmt.Errorf("open finance database: %w", err))
	}
	defer db.Close()

	if err := initializeDatabase(db); err != nil {
		panic(fmt.Errorf("initialize finance database: %w", err))
	}

	sheetSyncer, err := newGoogleSheetsSyncer(
		context.Background(),
		getenv("GOOGLE_SHEETS_SPREADSHEET_ID", ""),
		getenv("GOOGLE_SERVICE_ACCOUNT_JSON_BASE64", ""),
	)
	if err != nil {
		panic(fmt.Errorf("configure Google Sheets: %w", err))
	}

	s := newServer(db, &http.Client{Timeout: responseRequestTimeout}, getenv("SUMOPOD_RESPONSES_URL", defaultSumopodURL))
	s.recordSyncer = sheetSyncer
	if err := http.ListenAndServe(getenv("FINANCE_ADDR", defaultListenAddress), s.routes()); err != nil {
		panic(fmt.Errorf("serve finance API: %w", err))
	}
}

func newServer(db *sql.DB, httpClient *http.Client, sumopodResponsesURL string) *server {
	return &server{
		db:                  db,
		httpClient:          httpClient,
		sumopodResponsesURL: sumopodResponsesURL,
		recordSyncer:        disabledRecordSyncer{},
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /records", s.handleCreateRecord)
	mux.HandleFunc("GET /totals", s.handleTotals)
	mux.HandleFunc("GET /recap.xlsx", s.handleRecap)
	mux.HandleFunc("POST /openai/v1/responses", s.handleResponsesProxy)
	return mux
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxRequestBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("request must contain one JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
