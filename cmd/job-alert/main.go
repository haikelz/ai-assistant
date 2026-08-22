package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"ai-assistant/internal/domains/jobsearch/application"
	"ai-assistant/internal/domains/jobsearch/domain"
	"ai-assistant/internal/domains/jobsearch/infrastructure"
	"ai-assistant/internal/platform/config"
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
	assessor := infrastructure.NewAIAssessor(client, infrastructure.Config{Provider: cfg.AIProvider, Model: cfg.AIModel, SumopodAPIKey: cfg.SumopodAPIKey, OpenAIAPIKey: cfg.OpenAIAPIKey, GoogleAPIKey: cfg.GoogleAPIKey, SumopodURL: cfg.SumopodResponsesURL, OpenAIURL: cfg.OpenAIResponsesURL, GoogleURL: cfg.GoogleGenerativeURL})
	telegram := infrastructure.NewTelegram(client, cfg.TelegramBotToken, cfg.TelegramUserID, "")
	deliveries := []application.Delivery{{Name: "telegram", Messenger: telegram}}
	if cfg.WhatsAppRecipient != "" {
		deliveries = append(deliveries, application.Delivery{Name: "whatsapp", Messenger: infrastructure.NewLocalWhatsApp(client, cfg.WhatsAppGatewayURL)})
	}
	messenger := application.NewMultiMessenger(deliveries, log.Default())
	service := application.NewService([]application.Source{loggingSource{infrastructure.NewKitalulus(client, "")}, loggingSource{infrastructure.NewDealls(client, "")}}, assessor, messenger, log.Default())
	criteria := domain.Criteria{Positions: split(*keywords), Skills: split(*skills), Locations: split(*location), MaxYears: domain.ParseMaxYears(*experience), Halal: *halal, Interactive: strings.TrimSpace(*keywords) != ""}
	if *scheduled {
		settings := application.NewSettingsService(infrastructure.NewJSONAlertConfigStore(cfg.JobAlertConfigPath))
		var err error
		criteria, err = settings.ScheduledCriteria(context.Background())
		if err != nil {
			log.Fatalf("job-alert: load scheduled criteria: %v", err)
		}
		fmt.Fprintf(os.Stderr, "job-alert: scheduled criteria positions=%q skills=%q locations=%q max_years=%d halal=%t\n", criteria.Positions, criteria.Skills, criteria.Locations, criteria.MaxYears, criteria.Halal)
	}

	fmt.Fprintln(os.Stderr, "job-alert: fetching")
	result, err := service.Search(context.Background(), criteria)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "job-alert: matched kitalulus=%d dealls=%d\n", len(result.Kitalulus), len(result.Dealls))
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

func split(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			values = append(values, item)
		}
	}
	return values
}

type loggingSource struct{ application.Source }

func (s loggingSource) Fetch(ctx context.Context, criteria domain.Criteria) ([]domain.Job, error) {
	jobs, err := s.Source.Fetch(ctx, criteria)
	fmt.Fprintf(os.Stderr, "job-alert: fetched %s=%d\n", s.Name(), len(jobs))
	return jobs, err
}
