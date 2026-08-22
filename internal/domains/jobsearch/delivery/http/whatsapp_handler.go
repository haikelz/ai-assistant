package http

import (
	"context"

	"github.com/gofiber/fiber/v2"
)

type MessageSender interface {
	Send(context.Context, string) error
}

type WhatsAppHandler struct{ messenger MessageSender }

func NewWhatsAppHandler(messenger MessageSender) *WhatsAppHandler {
	return &WhatsAppHandler{messenger: messenger}
}

func (h *WhatsAppHandler) Register(router fiber.Router) {
	router.Post("/internal/whatsapp/send", h.send)
}

func (h *WhatsAppHandler) send(c *fiber.Ctx) error {
	if h.messenger == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "WhatsApp is disabled")
	}
	if len(c.Body()) > 256<<10 {
		return fiber.ErrRequestEntityTooLarge
	}
	var request struct {
		Message string `json:"message"`
	}
	if err := c.BodyParser(&request); err != nil || request.Message == "" {
		return fiber.ErrBadRequest
	}
	if err := h.messenger.Send(c.UserContext(), request.Message); err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}
