package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	aid "ai-assistant/internal/domains/ai/delivery/http"
	aiinfra "ai-assistant/internal/domains/ai/infrastructure"
	financeapp "ai-assistant/internal/domains/finance/application"
	financed "ai-assistant/internal/domains/finance/delivery/http"
	financeinfra "ai-assistant/internal/domains/finance/infrastructure"
	jobapp "ai-assistant/internal/domains/jobsearch/application"
	jobd "ai-assistant/internal/domains/jobsearch/delivery/http"
	jobinfra "ai-assistant/internal/domains/jobsearch/infrastructure"
	"ai-assistant/internal/platform/config"
	"github.com/gofiber/fiber/v2"
	_ "modernc.org/sqlite"
)

type Container struct {
	Config                   config.Config
	DB                       *sql.DB
	WhatsApp                 *jobinfra.WhatsAppGateway
	FinanceApp, JobSearchApp *fiber.App
}

func NewContainer(ctx context.Context, cfg config.Config) (*Container, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DatabasePath), 0o700); err != nil {
		return nil, fmt.Errorf("create finance database directory: %w", err)
	}
	db, err := sql.Open("sqlite", cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open finance database: %w", err)
	}
	if err := financeinfra.InitializeDatabase(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize finance database: %w", err)
	}
	syncer, err := financeinfra.NewGoogleSheetsSyncer(ctx, cfg.SpreadsheetID, cfg.ServiceAccountBase64)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("configure Google Sheets: %w", err)
	}
	client := &http.Client{Timeout: 120 * time.Second}
	financeService := financeapp.NewService(financeinfra.NewSQLiteRepository(db), syncer)
	proxy := aiinfra.NewSumopodProxy(client, cfg.SumopodResponsesURL)
	assessor := jobinfra.NewAIAssessor(client, jobinfra.Config{Provider: cfg.AIProvider, Model: cfg.AIModel, SumopodAPIKey: cfg.SumopodAPIKey, OpenAIAPIKey: cfg.OpenAIAPIKey, GoogleAPIKey: cfg.GoogleAPIKey, SumopodURL: cfg.SumopodResponsesURL, OpenAIURL: cfg.OpenAIResponsesURL, GoogleURL: cfg.GoogleGenerativeURL})
	telegram := jobinfra.NewTelegram(client, cfg.TelegramBotToken, cfg.TelegramUserID, "")
	deliveries := []jobapp.Delivery{{Name: "telegram", Messenger: telegram}}
	var whatsAppGateway *jobinfra.WhatsAppGateway
	var whatsAppMessenger *jobinfra.WhatsApp
	if cfg.WhatsAppRecipient != "" {
		if _, normalizeErr := jobinfra.NormalizeWhatsAppRecipient(cfg.WhatsAppRecipient); normalizeErr != nil {
			log.Printf("whatsapp: disabled: %v", normalizeErr)
		} else if gateway, gatewayErr := jobinfra.NewWhatsAppGateway(ctx, cfg.WhatsAppSessionPath, os.Stdout); gatewayErr != nil {
			log.Printf("whatsapp: disabled: %v", gatewayErr)
		} else if adapter, adapterErr := jobinfra.NewWhatsApp(gateway, cfg.WhatsAppRecipient); adapterErr != nil {
			_ = gateway.Close()
			log.Printf("whatsapp: disabled: %v", adapterErr)
		} else {
			whatsAppGateway = gateway
			whatsAppMessenger = adapter
			deliveries = append(deliveries, jobapp.Delivery{Name: "whatsapp", Messenger: adapter})
		}
	}
	messenger := jobapp.NewMultiMessenger(deliveries, log.Default())
	jobService := jobapp.NewService([]jobapp.Source{jobinfra.NewKitalulus(client, ""), jobinfra.NewDealls(client, "")}, assessor, messenger, log.Default())
	settingsService := jobapp.NewSettingsService(jobinfra.NewJSONAlertConfigStore(cfg.JobAlertConfigPath))

	mainAPI := newFiber()
	financed.NewHandler(financeService).Register(mainAPI)
	aid.NewHandler(proxy).Register(mainAPI)
	jobAPI := newFiber()
	jobAPI.Get("/health", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	jobd.NewHandler(jobService).Register(jobAPI)
	jobd.NewSettingsHandler(settingsService).Register(jobAPI)
	jobd.NewWhatsAppHandler(whatsAppMessenger).Register(jobAPI)
	return &Container{Config: cfg, DB: db, WhatsApp: whatsAppGateway, FinanceApp: mainAPI, JobSearchApp: jobAPI}, nil
}

func newFiber() *fiber.App {
	return fiber.New(fiber.Config{BodyLimit: 8 << 20, DisableStartupMessage: true})
}
func (c *Container) Close() error {
	var closeErrors []error
	if c.WhatsApp != nil {
		closeErrors = append(closeErrors, c.WhatsApp.Close())
	}
	if c.DB != nil {
		closeErrors = append(closeErrors, c.DB.Close())
	}
	return errors.Join(closeErrors...)
}
