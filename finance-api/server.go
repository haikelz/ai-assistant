package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
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
}

type recordRequest struct {
	Phone       string `json:"phone"`
	Type        string `json:"type"`
	Amount      int64  `json:"amount"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

type record struct {
	ID          int64     `json:"id"`
	Phone       string    `json:"phone"`
	Type        string    `json:"type"`
	Amount      int64     `json:"amount"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type totals struct {
	Modal   int64 `json:"modal"`
	Income  int64 `json:"income"`
	Expense int64 `json:"expense"`
	Money   int64 `json:"money"`
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

	s := newServer(db, &http.Client{Timeout: responseRequestTimeout}, getenv("SUMOPOD_RESPONSES_URL", defaultSumopodURL))
	if err := http.ListenAndServe(getenv("FINANCE_ADDR", defaultListenAddress), s.routes()); err != nil {
		panic(fmt.Errorf("serve finance API: %w", err))
	}
}

func newServer(db *sql.DB, httpClient *http.Client, sumopodResponsesURL string) *server {
	return &server{db: db, httpClient: httpClient, sumopodResponsesURL: sumopodResponsesURL}
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

func initializeDatabase(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			phone TEXT NOT NULL,
			type TEXT NOT NULL CHECK(type IN ('expense', 'income', 'modal')),
			amount INTEGER NOT NULL CHECK(amount > 0),
			category TEXT NOT NULL,
			description TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) handleCreateRecord(w http.ResponseWriter, r *http.Request) {
	var input recordRequest
	if err := decodeJSON(r, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateRecord(input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result, err := s.db.ExecContext(r.Context(), `
		INSERT INTO records (phone, type, amount, category, description)
		VALUES (?, ?, ?, ?, ?)`, input.Phone, input.Type, input.Amount, input.Category, input.Description)
	if err != nil {
		http.Error(w, "could not save record", http.StatusInternalServerError)
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		http.Error(w, "could not save record", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, record{ID: id, Phone: input.Phone, Type: input.Type, Amount: input.Amount, Category: input.Category, Description: input.Description})
}

func (s *server) handleTotals(w http.ResponseWriter, r *http.Request) {
	phone := strings.TrimSpace(r.URL.Query().Get("phone"))
	if phone == "" {
		http.Error(w, "phone is required", http.StatusBadRequest)
		return
	}

	total, err := s.lookupTotals(r.Context(), phone)
	if err != nil {
		http.Error(w, "could not calculate totals", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, total)
}

func (s *server) lookupTotals(ctx context.Context, phone string) (totals, error) {
	var total totals
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN type = 'modal' THEN amount END), 0),
			COALESCE(SUM(CASE WHEN type = 'income' THEN amount END), 0),
			COALESCE(SUM(CASE WHEN type = 'expense' THEN amount END), 0)
		FROM records WHERE phone = ?`, phone).Scan(&total.Modal, &total.Income, &total.Expense)
	if err != nil {
		return totals{}, err
	}
	total.Money = total.Modal + total.Income - total.Expense
	return total, nil
}

func (s *server) handleRecap(w http.ResponseWriter, r *http.Request) {
	phone := strings.TrimSpace(r.URL.Query().Get("phone"))
	if phone == "" {
		http.Error(w, "phone is required", http.StatusBadRequest)
		return
	}

	records, err := s.lookupRecords(r.Context(), phone)
	if err != nil {
		http.Error(w, "could not load records", http.StatusInternalServerError)
		return
	}

	file := excelize.NewFile()
	defer file.Close()
	sheet := file.GetSheetName(0)
	rows := [][]any{{"Tanggal", "Tipe", "Kategori", "Deskripsi", "Jumlah"}}
	for _, item := range records {
		rows = append(rows, []any{item.CreatedAt.In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02 15:04"), item.Type, item.Category, item.Description, item.Amount})
	}
	if err := file.SetSheetRow(sheet, "A1", &rows[0]); err != nil {
		http.Error(w, "could not create recap", http.StatusInternalServerError)
		return
	}
	for index := 1; index < len(rows); index++ {
		cell, err := excelize.CoordinatesToCellName(1, index+1)
		if err != nil || file.SetSheetRow(sheet, cell, &rows[index]) != nil {
			http.Error(w, "could not create recap", http.StatusInternalServerError)
			return
		}
	}
	_ = file.SetColWidth(sheet, "A", "D", 22)
	_ = file.SetColWidth(sheet, "E", "E", 16)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="finance-recap.xlsx"`)
	if err := file.Write(w); err != nil {
		return
	}
}

func (s *server) lookupRecords(ctx context.Context, phone string) ([]record, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, phone, type, amount, category, description, created_at
		FROM records WHERE phone = ? ORDER BY created_at, id`, phone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []record
	for rows.Next() {
		var item record
		if err := rows.Scan(&item.ID, &item.Phone, &item.Type, &item.Amount, &item.Category, &item.Description, &item.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, item)
	}
	return records, rows.Err()
}

func (s *server) handleResponsesProxy(w http.ResponseWriter, r *http.Request) {
	body := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	defer body.Close()

	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, s.sumopodResponsesURL, body)
	if err != nil {
		http.Error(w, "could not create Sumopod request", http.StatusInternalServerError)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	if authorization := r.Header.Get("Authorization"); authorization != "" {
		request.Header.Set("Authorization", authorization)
	}

	response, err := s.httpClient.Do(request)
	if err != nil {
		http.Error(w, "Sumopod request failed", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxRequestBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("request must contain one JSON object")
	}
	return nil
}

func validateRecord(input recordRequest) error {
	input.Phone = strings.TrimSpace(input.Phone)
	input.Type = strings.TrimSpace(input.Type)
	input.Category = strings.TrimSpace(input.Category)
	input.Description = strings.TrimSpace(input.Description)
	if input.Phone == "" || input.Type == "" || input.Category == "" || input.Description == "" || input.Amount <= 0 {
		return errors.New("phone, type, positive amount, category, and description are required")
	}
	if input.Type == "expense" && (input.Category == "Investasi" || input.Category == "Sumbangan" || input.Category == "Makan/Minum" || input.Category == "Lain - Lain") {
		return nil
	}
	if input.Type == "income" && input.Category == "Pendapatan" {
		return nil
	}
	if input.Type == "modal" && input.Category == "Modal" {
		return nil
	}
	return errors.New("invalid type or category")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
