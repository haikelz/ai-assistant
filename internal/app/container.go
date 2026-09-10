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
	jobproviders "ai-assistant/internal/domains/jobsearch/infrastructure/providers"
	"ai-assistant/internal/platform/config"
	"github.com/gofiber/fiber/v2"
	_ "modernc.org/sqlite"
)

type Container struct {
	Config                   config.Config
	DB                       *sql.DB
	WhatsApp                 *jobinfra.WhatsAppGateway
	FinanceApp, JobSearchApp *fiber.App
	jobSearchHandler         *jobd.JobSearchHandler
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
		return nil, errors.Join(fmt.Errorf("initialize finance database: %w", err), closeDatabase(db))
	}

	syncer, err := financeinfra.NewGoogleSheetsSyncer(ctx, cfg.SpreadsheetID, cfg.ServiceAccountBase64)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("configure Google Sheets: %w", err), closeDatabase(db))
	}

	client := &http.Client{Timeout: 120 * time.Second}
	financeService := financeapp.NewFinanceService(financeinfra.NewSQLiteRepository(db), syncer)
	proxy := aiinfra.NewSumopodProxy(client, cfg.SumopodResponsesURL)
	assessor := jobinfra.NewAIAssessor(client, jobinfra.AIProviderConfig{
		Provider:      cfg.AIProvider,
		Model:         cfg.AIModel,
		SumopodAPIKey: cfg.SumopodAPIKey,
		OpenAIAPIKey:  cfg.OpenAIAPIKey,
		GoogleAPIKey:  cfg.GoogleAPIKey,
		SumopodURL:    cfg.SumopodResponsesURL,
		OpenAIURL:     cfg.OpenAIResponsesURL,
		GoogleURL:     cfg.GoogleGenerativeURL,
	})
	telegram := jobinfra.NewTelegram(client, cfg.TelegramBotToken, cfg.TelegramUserID, "")
	jobSources := []jobapp.Source{
		jobinfra.NewKitalulus(client, ""),
		jobinfra.NewDealls(client, ""),
	}

	if cfg.LinkedInEnabled {
		linkedIn, linkedInErr := newLinkedInProvider(client, cfg)
		if linkedInErr != nil {
			return nil, errors.Join(fmt.Errorf("configure LinkedIn: %w", linkedInErr), closeDatabase(db))
		}
		jobSources = append(jobSources, linkedIn)
	}

	var whatsAppGateway *jobinfra.WhatsAppGateway
	var whatsAppMessenger *jobinfra.WhatsApp
	if cfg.WhatsAppRecipient != "" {
		if _, normalizeErr := jobinfra.NormalizeWhatsAppRecipient(cfg.WhatsAppRecipient); normalizeErr != nil {
			log.Printf("whatsapp: disabled: %v", normalizeErr)
		} else if gateway, gatewayErr := jobinfra.NewWhatsAppGateway(ctx, cfg.WhatsAppSessionPath, os.Stdout); gatewayErr != nil {
			log.Printf("whatsapp: disabled: %v", gatewayErr)
		} else if adapter, adapterErr := jobinfra.NewWhatsApp(gateway, cfg.WhatsAppRecipient); adapterErr != nil {
			if closeErr := gateway.Close(); closeErr != nil {
				log.Printf("whatsapp: close disabled gateway: %v", closeErr)
			}
			log.Printf("whatsapp: disabled: %v", adapterErr)
		} else {
			whatsAppGateway = gateway
			whatsAppMessenger = adapter
		}
	}

	messenger := jobapp.NewMultiMessenger(interactiveJobDeliveries(telegram), log.Default())
	jobService := jobapp.NewJobSearchService(jobSources, assessor, messenger, log.Default())
	settingsService := jobapp.NewSettingsService(jobinfra.NewJSONAlertConfigStore(cfg.JobAlertConfigPath))

	mainAPI := newFiber()
	financed.NewFinanceHandler(financeService).Register(mainAPI)
	aid.NewResponsesProxyHandler(proxy).Register(mainAPI)
	jobAPI := newFiber()
	jobAPI.Get("/health", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })

	jobSearchHandler, err := jobd.NewJobSearchHandler(ctx, jobService, log.Default())
	if err != nil {
		var cleanupErrors []error
		if whatsAppGateway != nil {
			cleanupErrors = append(cleanupErrors, whatsAppGateway.Close())
		}
		cleanupErrors = append(cleanupErrors, db.Close())
		return nil, errors.Join(fmt.Errorf("configure job-search handler: %w", err), errors.Join(cleanupErrors...))
	}
	jobSearchHandler.Register(jobAPI)
	jobd.NewSettingsHandler(settingsService).Register(jobAPI)
	jobd.NewWhatsAppHandler(whatsAppMessenger).Register(jobAPI)

	return &Container{
		Config:           cfg,
		DB:               db,
		WhatsApp:         whatsAppGateway,
		FinanceApp:       mainAPI,
		JobSearchApp:     jobAPI,
		jobSearchHandler: jobSearchHandler,
	}, nil
}

func newLinkedInProvider(client *http.Client, cfg config.Config) (*jobproviders.LinkedIn, error) {
	return jobproviders.NewLinkedIn(client, jobproviders.LinkedInConfig{
		Pages:        cfg.LinkedInPages,
		MaxDetails:   cfg.LinkedInMaxDetails,
		MaxQueries:   cfg.JobAlertMaxQueries,
		Distance:     cfg.LinkedInDistance,
		PostedWithin: time.Duration(cfg.LinkedInPostedWithinHours) * time.Hour,
		MinInterval:  500 * time.Millisecond,
		JobTypes:     cfg.LinkedInJobTypes,
		CompanyIDs:   cfg.LinkedInCompanyIDs,
	})
}

func interactiveJobDeliveries(telegram jobapp.Messenger) []jobapp.Delivery {
	return []jobapp.Delivery{
		{
			Name:      "telegram",
			Messenger: telegram,
		},
	}
}

func newFiber() *fiber.App {
	return fiber.New(fiber.Config{
		BodyLimit:             8 << 20,
		DisableStartupMessage: true,
	})
}

func (c *Container) Close() error {
	var closeErrors []error
	if c.jobSearchHandler != nil {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		closeErrors = append(closeErrors, c.jobSearchHandler.Shutdown(ctx))
		cancel()
	}
	if c.WhatsApp != nil {
		closeErrors = append(closeErrors, c.WhatsApp.Close())
	}
	if c.DB != nil {
		closeErrors = append(closeErrors, c.DB.Close())
	}
	return errors.Join(closeErrors...)
}

func closeDatabase(db *sql.DB) error {
	if err := db.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	return nil
}
