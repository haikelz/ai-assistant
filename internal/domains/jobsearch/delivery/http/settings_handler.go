package http

import (
	"context"
	"errors"
	"strings"

	"ai-assistant/internal/domains/jobsearch/domain"
	"github.com/gofiber/fiber/v2"
)

type Settings interface {
	Update(context.Context, string) (domain.AlertConfig, error)
	Current(context.Context) (domain.AlertConfig, error)
}

type SettingsHandler struct{ settings Settings }

func NewSettingsHandler(settings Settings) *SettingsHandler {
	return &SettingsHandler{settings: settings}
}

func (h *SettingsHandler) Register(router fiber.Router) {
	router.Get("/job-alert", h.current)
	router.Post("/job-alert", h.update)
}

func (h *SettingsHandler) update(c *fiber.Ctx) error {
	if len(c.Body()) > 4096 {
		return fiber.ErrRequestEntityTooLarge
	}
	var request struct {
		Query string `json:"query"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	config, err := h.settings.Update(c.UserContext(), request.Query)
	if errors.Is(err, domain.ErrInvalidAlertConfig) {
		return fiber.NewError(fiber.StatusBadRequest, strings.TrimPrefix(err.Error(), domain.ErrInvalidAlertConfig.Error()+": "))
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "gagal menyimpan konfigurasi job alert")
	}
	return c.JSON(fiber.Map{
		"status":     "updated",
		"message":    "Job alert diperbarui. Pencarian berjalan setiap hari pukul 03:00 WIB dengan label halal.",
		"schedule":   "03:00 Asia/Jakarta",
		"configured": true,
		"config":     config,
	})
}

func (h *SettingsHandler) current(c *fiber.Ctx) error {
	config, err := h.settings.Current(c.UserContext())
	if errors.Is(err, domain.ErrAlertConfigNotFound) {
		return c.JSON(fiber.Map{
			"status":     "not_configured",
			"message":    "Job alert belum dikonfigurasi. Kirim /job-alert posisi | skills | pengalaman | lokasi.",
			"schedule":   "03:00 Asia/Jakarta",
			"configured": false,
		})
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "gagal membaca konfigurasi job alert")
	}
	return c.JSON(fiber.Map{
		"status":     "configured",
		"message":    "Konfigurasi job alert aktif.",
		"schedule":   "03:00 Asia/Jakarta",
		"configured": true,
		"config":     config,
	})
}
