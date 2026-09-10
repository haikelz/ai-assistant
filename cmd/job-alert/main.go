package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai-assistant/internal/domains/jobsearch/application"
	"ai-assistant/internal/domains/jobsearch/domain"
	"ai-assistant/internal/domains/jobsearch/infrastructure"
	"ai-assistant/internal/domains/jobsearch/infrastructure/persistence"
	"ai-assistant/internal/domains/jobsearch/infrastructure/providers"
	"ai-assistant/internal/platform/config"
	_ "modernc.org/sqlite"
)

func main() {
	keywords := flag.String("keywords", "", "comma-separated job titles")
	location := flag.String("location", "", "comma-separated locations")
	skills := flag.String("skills", "", "comma-separated skills")
	experience := flag.String("experience", "", "experience range, e.g. 1-3")
	halal := flag.Bool("halal", false, "assess company business against halal criteria")
	scheduled := flag.Bool("scheduled", false, "use the persisted daily job-alert criteria")
	dryRun := flag.Bool("dry-run", false, "print instead of sending")
	flag.Parse()

	cfg := config.Load()
	client := &http.Client{Timeout: 120 * time.Second}
	assessor := infrastructure.NewAIAssessor(client, infrastructure.AIProviderConfig{
		Provider:      cfg.AIProvider,
		Model:         cfg.AIModel,
		SumopodAPIKey: cfg.SumopodAPIKey,
		OpenAIAPIKey:  cfg.OpenAIAPIKey,
		GoogleAPIKey:  cfg.GoogleAPIKey,
		SumopodURL:    cfg.SumopodResponsesURL,
		OpenAIURL:     cfg.OpenAIResponsesURL,
		GoogleURL:     cfg.GoogleGenerativeURL,
	})
	telegram := infrastructure.NewTelegram(client, cfg.TelegramBotToken, cfg.TelegramUserID, "")
	deliveries := []application.Delivery{
		{
			Name:      "telegram",
			Messenger: telegram,
		},
	}

	if *scheduled && cfg.WhatsAppRecipient != "" {
		deliveries = append(deliveries, application.Delivery{
			Name:      "whatsapp",
			Messenger: infrastructure.NewLocalWhatsApp(client, cfg.WhatsAppGatewayURL),
		})
	}

	if *scheduled && cfg.MailTo != "" {
		email := infrastructure.NewEmail(infrastructure.EmailConfig{
			Mailer:     cfg.MailMailer,
			Username:   cfg.MailUsername,
			Password:   cfg.MailPassword,
			Host:       cfg.MailHost,
			Port:       cfg.MailPort,
			Encryption: cfg.MailEncryption,
			From:       cfg.MailFrom,
			To:         cfg.MailTo,
		})
		deliveries = append(deliveries, application.Delivery{
			Name:      "email",
			Messenger: email,
		})
	}

	messenger := application.NewMultiMessenger(deliveries, log.Default())
	criteria := domain.Criteria{
		Positions:   split(*keywords),
		Skills:      split(*skills),
		Locations:   split(*location),
		MaxYears:    domain.ParseMaxYears(*experience),
		Halal:       *halal,
		Interactive: strings.TrimSpace(*keywords) != "",
	}

	if *scheduled {
		settings := application.NewSettingsService(infrastructure.NewJSONAlertConfigStore(cfg.JobAlertConfigPath))
		var err error
		criteria, err = settings.ScheduledCriteria(context.Background())
		if err != nil {
			log.Fatalf("job-alert: load scheduled criteria: %v", err)
		}
		fmt.Fprintf(os.Stderr, "job-alert: scheduled criteria positions=%q skills=%q locations=%q max_years=%d halal=%t\n", criteria.Positions, criteria.Skills, criteria.Locations, criteria.MaxYears, criteria.Halal)
	}

	if *scheduled && cfg.JobAlertPipelineEnabled {
		if err := runCurated(context.Background(), cfg, client, assessor, messenger, criteria, *dryRun); err != nil {
			log.Fatal(err)
		}
		return
	}

	sources := []application.Source{
		loggingSource{Source: infrastructure.NewKitalulus(client, "")},
		loggingSource{Source: infrastructure.NewDealls(client, "")},
	}

	if cfg.LinkedInEnabled {
		linkedIn, err := newLinkedInProvider(client, cfg)
		if err != nil {
			log.Fatalf("job-alert: configure LinkedIn: %v", err)
		}
		sources = append(sources, loggingSource{Source: linkedIn})
	}
	service := application.NewJobSearchService(sources, assessor, messenger, log.Default())
	fmt.Fprintln(os.Stderr, "job-alert: fetching")
	result, err := service.Search(context.Background(), criteria)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "job-alert: matched kitalulus=%d dealls=%d linkedin=%d\n", len(result.Kitalulus), len(result.Dealls), len(result.LinkedIn))
	greeting := "Selamat pagi! ☀️ Berikut update lowongan kerja terbaru hari ini:"
	if criteria.Interactive {
		greeting = "Berikut hasil pencarian lowongan kerja:"
	}
	message := domain.FormatMessage(greeting, result)
	if *dryRun {
		fmt.Println(message)
		return
	}
	if err := messenger.Send(context.Background(), message); err != nil {
		log.Fatal(err)
	}
}

func runCurated(
	ctx context.Context,
	cfg config.Config,
	client *http.Client,
	assessor *infrastructure.AIAssessor,
	messenger application.Messenger,
	criteria domain.Criteria,
	dryRun bool,
) error {
	if err := os.MkdirAll(filepath.Dir(cfg.JobAlertDBPath), 0o700); err != nil {
		return fmt.Errorf("create job-alert database directory: %w", err)
	}
	database, err := sql.Open("sqlite", cfg.JobAlertDBPath)
	if err != nil {
		return fmt.Errorf("open job-alert database: %w", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	store := persistence.NewSQLiteJobStore(database)
	if err := store.Initialize(ctx); err != nil {
		return err
	}

	jobProviders := []application.JobProvider{
		providers.NewKitalulus(client, ""),
		providers.NewDealls(client, ""),
	}

	if cfg.GlintsEnabled {
		jobProviders = append(jobProviders, providers.NewGlints(client, "", 2*time.Second))
	}
	if cfg.LinkedInEnabled {
		linkedIn, err := newLinkedInProvider(client, cfg)
		if err != nil {
			return fmt.Errorf("configure LinkedIn: %w", err)
		}
		jobProviders = append(jobProviders, linkedIn)
	}
	if criteria.MinMatchScore <= 0 {
		criteria.MinMatchScore = cfg.JobAlertMinMatchScore
	}
	providerTimeout := 10 * time.Second
	if cfg.LinkedInEnabled {
		providerTimeout = 45 * time.Second
	}
	ingestion := application.NewIngestionPipeline(
		application.NewSearchPlanner(cfg.JobAlertMaxQueries),
		jobProviders,
		store,
		providerTimeout,
		4,
		log.Default(),
	)
	classifier := infrastructure.NewBatchClassifier(assessor, cfg.JobAlertBatchSize)
	service := application.NewCuratedService(
		ingestion,
		classifier,
		application.NewMatchEngine(cfg.JobAlertMinMatchScore),
		store,
		messenger,
		log.Default(),
	)

	fmt.Fprintf(os.Stderr, "job-alert: curated pipeline fetching providers=%d glints=%t linkedin=%t dry_run=%t\n", len(jobProviders), cfg.GlintsEnabled, cfg.LinkedInEnabled, dryRun)
	message, run, err := service.Run(ctx, criteria, !dryRun, !dryRun)
	fmt.Fprintf(os.Stderr, "job-alert: run=%s status=%s fetched=%d new=%d updated=%d filtered=%d classified=%d matched=%d\n", run.ID, run.Status, run.JobsFetched, run.JobsNew, run.JobsUpdated, run.JobsFiltered, run.JobsClassified, run.JobsMatched)
	if dryRun {
		fmt.Println(message)
	}
	return err
}

func newLinkedInProvider(client *http.Client, cfg config.Config) (*providers.LinkedIn, error) {
	return providers.NewLinkedIn(client, providers.LinkedInConfig{
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

func split(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			values = append(values, item)
		}
	}
	return values
}

type loggingSource struct {
	application.Source
}

func (s loggingSource) Fetch(ctx context.Context, criteria domain.Criteria) ([]domain.Job, error) {
	jobs, err := s.Source.Fetch(ctx, criteria)
	fmt.Fprintf(os.Stderr, "job-alert: fetched %s=%d\n", s.Name(), len(jobs))
	return jobs, err
}
