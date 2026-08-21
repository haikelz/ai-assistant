package http

import (
	"context"
	"errors"

	"ai-assistant/internal/domains/ai/domain"
	"github.com/gofiber/fiber/v2"
)

type Proxy interface {
	Forward(context.Context, string, []byte) (domain.ProxyResponse, error)
}

type Handler struct{ proxy Proxy }

func NewHandler(proxy Proxy) *Handler           { return &Handler{proxy: proxy} }
func (h *Handler) Register(router fiber.Router) { router.Post("/openai/v1/responses", h.responses) }
func (h *Handler) responses(c *fiber.Ctx) error {
	response, err := h.proxy.Forward(c.UserContext(), c.Get(fiber.HeaderAuthorization), c.Body())
	if err != nil {
		if errors.Is(err, domain.ErrInvalidRequest) {
			return fiber.NewError(fiber.StatusBadRequest, "could not prepare Sumopod request")
		}
		return fiber.NewError(fiber.StatusBadGateway, "Sumopod request failed")
	}
	if response.ContentType != "" {
		c.Set(fiber.HeaderContentType, response.ContentType)
	}
	return c.Status(response.Status).Send(response.Body)
}
