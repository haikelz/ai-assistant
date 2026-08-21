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
	dryRun := flag.Bool("dry-run", false, "print instead of sending")
	flag.Parse()

	cfg := config.Load()
	client := &http.Client{Timeout: 120 * time.Second}
	assessor := infrastructure.NewAIAssessor(client, infrastructure.Config{Provider: cfg.AIProvider, Model: cfg.AIModel, SumopodAPIKey: cfg.SumopodAPIKey, OpenAIAPIKey: cfg.OpenAIAPIKey, GoogleAPIKey: cfg.GoogleAPIKey, SumopodURL: cfg.SumopodResponsesURL, OpenAIURL: cfg.OpenAIResponsesURL, GoogleURL: cfg.GoogleGenerativeURL})
	messenger := infrastructure.NewTelegram(client, cfg.TelegramBotToken, cfg.TelegramUserID, "")
	service := application.NewService([]application.Source{loggingSource{infrastructure.NewKitalulus(client, "")}, loggingSource{infrastructure.NewDealls(client, "")}}, assessor, messenger, log.Default())
	criteria := domain.Criteria{Positions: split(*keywords), Skills: split(*skills), Locations: split(*location), MaxYears: domain.ParseMaxYears(*experience), Halal: *halal, Interactive: strings.TrimSpace(*keywords) != ""}

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
