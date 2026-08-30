package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type pipelineProvider struct {
	source string
	jobs   []domain.RawJob
	err    error
}

func (p pipelineProvider) Source() string { return p.source }
func (p pipelineProvider) Search(context.Context, domain.SearchQuery) ([]domain.RawJob, error) {
	return p.jobs, p.err
}
func (p pipelineProvider) HealthCheck(context.Context) error { return p.err }

type pipelineStore struct {
	outcomes []domain.UpsertOutcome
	index    int
}

func (s *pipelineStore) Upsert(context.Context, domain.NormalizedJob) (domain.UpsertOutcome, error) {
	outcome := s.outcomes[s.index]
	s.index++
	return outcome, nil
}

func TestIngestionPipelineIsolatesProvidersAndSkipsUnchangedJobs(t *testing.T) {
	store := &pipelineStore{outcomes: []domain.UpsertOutcome{domain.UpsertNew, domain.UpsertUnchanged}}
	providers := []JobProvider{
		pipelineProvider{source: "dealls", jobs: []domain.RawJob{{Source: "dealls", ExternalID: "1", URL: "https://x/1", Title: "Software Engineer", Location: "Jakarta"}, {Source: "dealls", ExternalID: "2", URL: "https://x/2", Title: "Software Engineer", Location: "Jakarta"}}},
		pipelineProvider{source: "broken", err: errors.New("down")},
	}
	pipeline := NewIngestionPipeline(NewSearchPlanner(1), providers, store, time.Second, 2, nil)
	pipeline.now = func() time.Time { return time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC) }
	result, err := pipeline.Run(t.Context(), domain.Criteria{Positions: []string{"Software Engineer"}, Locations: []string{"Jakarta"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 2 || result.New != 1 || result.Skipped != 1 || len(result.Jobs) != 1 {
		t.Fatalf("result=%#v", result)
	}
}

func TestIngestionPipelineKeepsPartialJobsFromFailedProvider(t *testing.T) {
	provider := pipelineProvider{source: "linkedin", jobs: []domain.RawJob{{Source: "linkedin", ExternalID: "1", URL: "https://example/1", Title: "Software Engineer"}}, err: errors.New("blocked after first page")}
	pipeline := NewIngestionPipeline(NewSearchPlanner(1), []JobProvider{provider}, nil, time.Second, 1, nil)
	result, err := pipeline.Run(t.Context(), domain.Criteria{Positions: []string{"Software Engineer"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 1 || len(result.Jobs) != 1 || result.Jobs[0].Source != "linkedin" {
		t.Fatalf("result=%#v", result)
	}
}
