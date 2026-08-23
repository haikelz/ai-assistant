package http

import (
	"context"
	"strings"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
	"github.com/gofiber/fiber/v2"
)

type Deliverer interface {
	AcknowledgeSearch(context.Context) error
	SearchAndDeliver(context.Context, domain.Criteria) error
}
type Handler struct{ service Deliverer }

func NewHandler(service Deliverer) *Handler     { return &Handler{service: service} }
func (h *Handler) Register(router fiber.Router) { router.Post("/loker", h.Handle) }
func (h *Handler) Handle(c *fiber.Ctx) error {
	if len(c.Body()) > 4096 {
		return fiber.ErrRequestEntityTooLarge
	}
	var request struct {
		Query string `json:"query"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	if strings.TrimSpace(request.Query) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "missing query field")
	}
	criteria := domain.ParseQuery(request.Query)
	acknowledgementContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	_ = h.service.AcknowledgeSearch(acknowledgementContext)
	cancel()
	go func() { _ = h.service.SearchAndDeliver(context.Background(), criteria) }()
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"status": "searching"})
}
