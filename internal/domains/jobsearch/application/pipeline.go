package application

import (
	"context"
	"log"
	"sync"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type JobProvider interface {
	Source() string
	Search(context.Context, domain.SearchQuery) ([]domain.RawJob, error)
	HealthCheck(context.Context) error
}

type QueryAwareProvider interface {
	SupportsQueries() bool
}

type JobStore interface {
	Upsert(context.Context, domain.NormalizedJob) (domain.UpsertOutcome, error)
}

type PipelineResult struct {
	Jobs                           []domain.NormalizedJob
	Fetched, New, Updated, Skipped int
}

type IngestionPipeline struct {
	planner     *SearchPlanner
	providers   []JobProvider
	store       JobStore
	prefilter   PreFilter
	timeout     time.Duration
	concurrency int
	now         func() time.Time
	logger      *log.Logger
}

func NewIngestionPipeline(
	planner *SearchPlanner,
	providers []JobProvider,
	store JobStore,
	timeout time.Duration,
	concurrency int,
	logger *log.Logger,
) *IngestionPipeline {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if concurrency < 1 {
		concurrency = 4
	}
	return &IngestionPipeline{
		planner:     planner,
		providers:   providers,
		store:       store,
		prefilter:   PreFilter{},
		timeout:     timeout,
		concurrency: concurrency,
		now:         time.Now,
		logger:      logger,
	}
}

func (p *IngestionPipeline) Run(ctx context.Context, criteria domain.Criteria, deduplicate bool) (PipelineResult, error) {
	queries := p.planner.Plan(criteria)
	type response struct {
		jobs []domain.RawJob
		err  error
		name string
	}
	requests := 0
	for _, provider := range p.providers {
		requests += len(queries)
		if queryAware, ok := provider.(QueryAwareProvider); ok && !queryAware.SupportsQueries() {
			requests -= len(queries) - 1
		}
	}
	responses := make(chan response, requests)
	semaphore := make(chan struct{}, p.concurrency)
	var wait sync.WaitGroup
	for _, provider := range p.providers {
		providerQueries := queries
		if queryAware, ok := provider.(QueryAwareProvider); ok && !queryAware.SupportsQueries() {
			providerQueries = queries[:1]
		}
		for _, query := range providerQueries {
			wait.Add(1)
			go func(provider JobProvider, query domain.SearchQuery) {
				defer wait.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()
				requestCtx, cancel := context.WithTimeout(ctx, p.timeout)
				defer cancel()
				jobs, err := provider.Search(requestCtx, query)
				responses <- response{jobs: jobs, err: err, name: provider.Source()}
			}(provider, query)
		}
	}
	wait.Wait()
	close(responses)

	seen := map[string]bool{}
	var result PipelineResult
	var normalized []domain.NormalizedJob
	now := p.now().UTC()
	for response := range responses {
		if response.err != nil {
			if p.logger != nil {
				p.logger.Printf("jobsearch: %s search: %v", response.name, response.err)
			}
			if len(response.jobs) == 0 {
				continue
			}
		}
		result.Fetched += len(response.jobs)
		for _, raw := range response.jobs {
			job := NormalizeJob(raw, now)
			if seen[job.ID] {
				continue
			}
			seen[job.ID] = true
			if deduplicate && p.store != nil {
				outcome, err := p.store.Upsert(ctx, job)
				if err != nil {
					return PipelineResult{}, err
				}
				switch outcome {
				case domain.UpsertNew:
					result.New++
				case domain.UpsertUpdated:
					result.Updated++
				case domain.UpsertUnchanged:
					result.Skipped++
					continue
				}
			}
			normalized = append(normalized, job)
		}
	}
	result.Jobs = p.prefilter.Apply(normalized, criteria)
	return result, nil
}
