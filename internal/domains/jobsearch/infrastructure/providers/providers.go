package providers

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"ai-assistant/internal/domains/jobsearch/domain"
	legacy "ai-assistant/internal/domains/jobsearch/infrastructure"
)

type Kitalulus struct{ source *legacy.Kitalulus }

func NewKitalulus(client *http.Client, baseURL string) *Kitalulus {
	return &Kitalulus{source: legacy.NewKitalulus(client, baseURL)}
}
func (*Kitalulus) Source() string        { return "kitalulus" }
func (*Kitalulus) SupportsQueries() bool { return true }
func (p *Kitalulus) HealthCheck(ctx context.Context) error {
	_, err := p.Search(ctx, domain.SearchQuery{Keyword: "software engineer"})
	return err
}
func (p *Kitalulus) Search(ctx context.Context, query domain.SearchQuery) ([]domain.RawJob, error) {
	jobs, err := p.source.Fetch(ctx, domain.Criteria{Positions: []string{query.Keyword}, Locations: nonempty(query.Location)})
	return toRawJobs(jobs), err
}

type Dealls struct{ source *legacy.Dealls }

func NewDealls(client *http.Client, baseURL string) *Dealls {
	return &Dealls{source: legacy.NewDealls(client, baseURL)}
}
func (*Dealls) Source() string        { return "dealls" }
func (*Dealls) SupportsQueries() bool { return true }
func (p *Dealls) HealthCheck(ctx context.Context) error {
	_, err := p.Search(ctx, domain.SearchQuery{Keyword: "software engineer"})
	return err
}
func (p *Dealls) Search(ctx context.Context, query domain.SearchQuery) ([]domain.RawJob, error) {
	jobs, err := p.source.Fetch(ctx, domain.Criteria{Positions: []string{query.Keyword}, Locations: nonempty(query.Location)})
	return toRawJobs(jobs), err
}

func toRawJobs(jobs []domain.Job) []domain.RawJob {
	raw := make([]domain.RawJob, 0, len(jobs))
	for _, job := range jobs {
		externalID := ""
		if parsed, err := url.Parse(job.URL); err == nil {
			parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			if len(parts) > 0 {
				externalID = parts[len(parts)-1]
			}
		}
		raw = append(raw, domain.RawJob{Source: job.Source, ExternalID: externalID, URL: job.URL, Title: job.Title, Company: job.Company, Location: job.Location, SalaryText: job.Salary, EmploymentType: job.Type, ExperienceText: job.Experience, Skills: splitSkills(job.Skills)})
	}
	return raw
}

func splitSkills(value string) []string {
	var skills []string
	for _, skill := range strings.Split(value, ",") {
		if skill = strings.TrimSpace(skill); skill != "" {
			skills = append(skills, skill)
		}
	}
	return skills
}

func nonempty(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{value}
}
