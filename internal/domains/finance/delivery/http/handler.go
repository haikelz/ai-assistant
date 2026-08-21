package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"ai-assistant/internal/domains/finance/domain"
	"github.com/gofiber/fiber/v2"
	"github.com/xuri/excelize/v2"
)

type Service interface {
	Create(context.Context, domain.RecordInput) (domain.Record, domain.SyncStatus, error)
	Totals(context.Context, string) (domain.Totals, error)
	Records(context.Context, string) ([]domain.Record, error)
	Ping(context.Context) error
}

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }
func (h *Handler) Register(router fiber.Router) {
	router.Get("/health", h.health)
	router.Post("/records", h.create)
	router.Get("/totals", h.totals)
	router.Get("/recap.xlsx", h.recap)
}

func (h *Handler) health(c *fiber.Ctx) error {
	if err := h.service.Ping(c.UserContext()); err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "database unavailable")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) create(c *fiber.Ctx) error {
	var input domain.RecordInput
	decoder := json.NewDecoder(bytes.NewReader(c.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON")
	}
	record, status, err := h.service.Create(c.UserContext(), input)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidRecord) {
			return fiber.NewError(fiber.StatusBadRequest, strings.TrimPrefix(err.Error(), domain.ErrInvalidRecord.Error()+": "))
		}
		return fiber.NewError(fiber.StatusInternalServerError, "could not save record")
	}
	return c.Status(fiber.StatusCreated).JSON(struct {
		domain.Record
		SpreadsheetSyncStatus domain.SyncStatus `json:"spreadsheet_sync_status"`
	}{record, status})
}

func (h *Handler) totals(c *fiber.Ctx) error {
	phone := strings.TrimSpace(c.Query("phone"))
	if phone == "" {
		return fiber.NewError(fiber.StatusBadRequest, "phone is required")
	}
	totals, err := h.service.Totals(c.UserContext(), phone)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "could not calculate totals")
	}
	return c.JSON(totals)
}

func (h *Handler) recap(c *fiber.Ctx) error {
	phone := strings.TrimSpace(c.Query("phone"))
	if phone == "" {
		return fiber.NewError(fiber.StatusBadRequest, "phone is required")
	}
	records, err := h.service.Records(c.UserContext(), phone)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "could not load records")
	}
	file := excelize.NewFile()
	defer file.Close()
	sheet := file.GetSheetName(0)
	rows := [][]any{{"Tanggal", "Tipe", "Kategori", "Deskripsi", "Jumlah"}}
	for _, item := range records {
		rows = append(rows, []any{item.CreatedAt.In(fixedWIB).Format("2006-01-02 15:04"), item.Type, item.Category, item.Description, item.Amount})
	}
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := file.SetSheetRow(sheet, cell, &row); err != nil {
			return fiber.NewError(500, "could not create recap")
		}
	}
	_ = file.SetColWidth(sheet, "A", "D", 22)
	_ = file.SetColWidth(sheet, "E", "E", 16)
	var output bytes.Buffer
	if err := file.Write(&output); err != nil {
		return fiber.NewError(500, "could not create recap")
	}
	c.Set(fiber.HeaderContentType, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="finance-recap.xlsx"`)
	return c.Send(output.Bytes())
}

var fixedWIB = mustWIB()

func mustWIB() *time.Location { return time.FixedZone("WIB", 7*60*60) }
