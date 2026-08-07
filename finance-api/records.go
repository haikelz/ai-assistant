package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
)

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

type createRecordResponse struct {
	record
	SpreadsheetSyncStatus spreadsheetSyncStatus `json:"spreadsheet_sync_status"`
}

type totals struct {
	Modal   int64 `json:"modal"`
	Income  int64 `json:"income"`
	Expense int64 `json:"expense"`
	Money   int64 `json:"money"`
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

	createdAt := time.Now().UTC()
	result, err := s.db.ExecContext(r.Context(), `
		INSERT INTO records (phone, type, amount, category, description, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, input.Phone, input.Type, input.Amount, input.Category, input.Description, createdAt)
	if err != nil {
		http.Error(w, "could not save record", http.StatusInternalServerError)
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		http.Error(w, "could not save record", http.StatusInternalServerError)
		return
	}

	saved := record{ID: id, Phone: input.Phone, Type: input.Type, Amount: input.Amount, Category: input.Category, Description: input.Description, CreatedAt: createdAt}
	status, err := s.recordSyncer.Sync(r.Context(), saved)
	if err != nil {
		status = spreadsheetSyncStatusPending
	}
	writeJSON(w, http.StatusCreated, createRecordResponse{record: saved, SpreadsheetSyncStatus: status})
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
