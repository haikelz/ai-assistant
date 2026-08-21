package infrastructure

import (
	"context"
	"database/sql"
	"time"

	"ai-assistant/internal/domains/finance/domain"
)

type SQLiteRepository struct{ db *sql.DB }

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository { return &SQLiteRepository{db: db} }

func InitializeDatabase(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		phone TEXT NOT NULL,
		type TEXT NOT NULL CHECK(type IN ('expense', 'income', 'modal')),
		amount INTEGER NOT NULL CHECK(amount > 0),
		category TEXT NOT NULL,
		description TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func (r *SQLiteRepository) Create(ctx context.Context, input domain.RecordInput, createdAt time.Time) (domain.Record, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO records (phone, type, amount, category, description, created_at) VALUES (?, ?, ?, ?, ?, ?)`, input.Phone, input.Type, input.Amount, input.Category, input.Description, createdAt)
	if err != nil {
		return domain.Record{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.Record{}, err
	}
	return domain.Record{ID: id, Phone: input.Phone, Type: input.Type, Amount: input.Amount, Category: input.Category, Description: input.Description, CreatedAt: createdAt}, nil
}

func (r *SQLiteRepository) Totals(ctx context.Context, phone string) (domain.Totals, error) {
	var total domain.Totals
	err := r.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN type = 'modal' THEN amount END), 0),
		COALESCE(SUM(CASE WHEN type = 'income' THEN amount END), 0),
		COALESCE(SUM(CASE WHEN type = 'expense' THEN amount END), 0)
		FROM records WHERE phone = ?`, phone).Scan(&total.Modal, &total.Income, &total.Expense)
	total.Money = total.Modal + total.Income - total.Expense
	return total, err
}

func (r *SQLiteRepository) Records(ctx context.Context, phone string) ([]domain.Record, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, phone, type, amount, category, description, created_at FROM records WHERE phone = ? ORDER BY created_at, id`, phone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []domain.Record
	for rows.Next() {
		var item domain.Record
		if err := rows.Scan(&item.ID, &item.Phone, &item.Type, &item.Amount, &item.Category, &item.Description, &item.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, item)
	}
	return records, rows.Err()
}

func (r *SQLiteRepository) Ping(ctx context.Context) error { return r.db.PingContext(ctx) }
