package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidRecord = errors.New("invalid finance record")

type RecordInput struct {
	Phone       string `json:"phone"`
	Type        string `json:"type"`
	Amount      int64  `json:"amount"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

type Record struct {
	ID          int64     `json:"id"`
	Phone       string    `json:"phone"`
	Type        string    `json:"type"`
	Amount      int64     `json:"amount"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type Totals struct {
	Modal   int64 `json:"modal"`
	Income  int64 `json:"income"`
	Expense int64 `json:"expense"`
	Money   int64 `json:"money"`
}

type SyncStatus string

const (
	SyncDisabled SyncStatus = "disabled"
	SyncSynced   SyncStatus = "synced"
	SyncPending  SyncStatus = "pending"
)

func (i *RecordInput) Validate() error {
	i.Phone = strings.TrimSpace(i.Phone)
	i.Type = strings.TrimSpace(i.Type)
	i.Category = strings.TrimSpace(i.Category)
	i.Description = strings.TrimSpace(i.Description)
	if i.Phone == "" || i.Type == "" || i.Category == "" || i.Description == "" || i.Amount <= 0 {
		return fmt.Errorf("%w: phone, type, positive amount, category, and description are required", ErrInvalidRecord)
	}
	valid := (i.Type == "expense" && (i.Category == "Investasi" || i.Category == "Sumbangan" || i.Category == "Makan/Minum" || i.Category == "Lain - Lain")) ||
		(i.Type == "income" && i.Category == "Pendapatan") || (i.Type == "modal" && i.Category == "Modal")
	if !valid {
		return fmt.Errorf("%w: invalid type or category", ErrInvalidRecord)
	}
	return nil
}
