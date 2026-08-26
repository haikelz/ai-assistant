package http

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
	"github.com/gofiber/fiber/v2"
)

type Deliverer interface {
	AcknowledgeSearch(context.Context) error
	SearchAndDeliver(context.Context, domain.Criteria) error
}
type Handler struct {
	service Deliverer
	logger  *log.Logger
	ctx     context.Context
	cancel  context.CancelFunc

	mu         sync.Mutex
	active     int
	closed     bool
	doneClosed bool
	done       chan struct{}
}

func NewHandler(parent context.Context, service Deliverer, logger *log.Logger) (*Handler, error) {
	if parent == nil {
		return nil, errors.New("parent context is required")
	}
	if service == nil {
		return nil, errors.New("deliverer is required")
	}
	if logger == nil {
		logger = log.Default()
	}
	ctx, cancel := context.WithCancel(parent)
	return &Handler{service: service, logger: logger, ctx: ctx, cancel: cancel, done: make(chan struct{})}, nil
}

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
	acknowledgementContext, cancel := context.WithTimeout(h.ctx, 15*time.Second)
	defer cancel()
	if err := h.service.AcknowledgeSearch(acknowledgementContext); err != nil {
		h.logger.Printf("jobsearch: acknowledge interactive search: %v", err)
	}
	if !h.startSearch(criteria) {
		return fiber.ErrServiceUnavailable
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"status": "searching"})
}

func (h *Handler) startSearch(criteria domain.Criteria) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.ctx.Err() != nil {
		return false
	}
	h.active++
	go func() {
		defer h.finishSearch()
		if err := h.service.SearchAndDeliver(h.ctx, criteria); err != nil && !errors.Is(err, context.Canceled) {
			h.logger.Printf("jobsearch: deliver interactive search: %v", err)
		}
	}()
	return true
}

func (h *Handler) finishSearch() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.active--
	h.closeDoneIfIdle()
}

func (h *Handler) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("shutdown context is required")
	}
	h.mu.Lock()
	if !h.closed {
		h.closed = true
		h.cancel()
	}
	h.closeDoneIfIdle()
	done := h.done
	h.mu.Unlock()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for interactive searches: %w", ctx.Err())
	}
}

func (h *Handler) closeDoneIfIdle() {
	if h.closed && h.active == 0 && !h.doneClosed {
		close(h.done)
		h.doneClosed = true
	}
}
