package application

import (
	"context"
	"log"
	"strings"
	"sync"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type Source interface {
	Name() string
	Fetch(context.Context, domain.Criteria) ([]domain.Job, error)
}

type Assessor interface {
	Assess(context.Context, []domain.Job) ([]domain.Job, error)
}

type Messenger interface {
	Send(context.Context, string) error
}

type JobSearchService struct {
	sources   []Source
	assessor  Assessor
	messenger Messenger
	logger    *log.Logger
}

func NewJobSearchService(
	sources []Source,
	assessor Assessor,
	messenger Messenger,
	logger *log.Logger,
) *JobSearchService {
	return &JobSearchService{
		sources:   append([]Source(nil), sources...),
		assessor:  assessor,
		messenger: messenger,
		logger:    logger,
	}
}

const SearchAcknowledgement = "Mencari lowongan di Kitalulus dan Dealls. Hasil akan dikirim ke chat kamu."

func (s *JobSearchService) AcknowledgeSearch(ctx context.Context) error {
	if s.messenger == nil {
		return nil
	}

	return s.messenger.Send(ctx, s.searchAcknowledgement())
}

func (s *JobSearchService) searchAcknowledgement() string {
	names := make([]string, 0, len(s.sources))
	for _, source := range s.sources {
		switch source.Name() {
		case "kitalulus":
			names = append(names, "Kitalulus")
		case "dealls":
			names = append(names, "Dealls")
		case "linkedin":
			names = append(names, "LinkedIn")
		}
	}

	if len(names) == 0 {
		return SearchAcknowledgement
	}
	return "Mencari lowongan di " + joinIndonesian(names) + ". Hasil akan dikirim ke chat kamu."
}

func joinIndonesian(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	case 2:
		return values[0] + " dan " + values[1]
	default:
		return strings.Join(values[:len(values)-1], ", ") + ", dan " + values[len(values)-1]
	}
}

type sourceFetchResult struct {
	name string
	jobs []domain.Job
}

func (s *JobSearchService) Search(ctx context.Context, criteria domain.Criteria) (domain.Result, error) {
	results := make(chan sourceFetchResult, len(s.sources))

	var waitGroup sync.WaitGroup
	for _, source := range s.sources {
		waitGroup.Add(1)
		go func(source Source) {
			defer waitGroup.Done()

			jobs, err := source.Fetch(ctx, criteria)
			if err != nil {
				if s.logger != nil {
					s.logger.Printf("jobsearch: %s fetch: %v", source.Name(), err)
				}
				if len(jobs) == 0 {
					jobs = nil
				}
			}

			results <- sourceFetchResult{
				name: source.Name(),
				jobs: domain.FilterAndSort(jobs, criteria, 20),
			}
		}(source)
	}

	waitGroup.Wait()
	close(results)

	var result domain.Result
	for fetched := range results {
		switch fetched.name {
		case "kitalulus":
			result.Kitalulus = fetched.jobs
		case "dealls":
			result.Dealls = fetched.jobs
		case "linkedin":
			result.LinkedIn = fetched.jobs
			result.LinkedInIncluded = true
		}
	}

	if criteria.Halal && s.assessor != nil {
		kitalulusCount := len(result.Kitalulus)
		deallsCount := len(result.Dealls)
		allJobs := append([]domain.Job{}, result.Kitalulus...)
		allJobs = append(allJobs, result.Dealls...)
		allJobs = append(allJobs, result.LinkedIn...)

		for index := range allJobs {
			allJobs[index].HalalStatus = domain.HalalStatusNeedsReview
		}

		copy(result.Kitalulus, allJobs[:kitalulusCount])
		copy(result.Dealls, allJobs[kitalulusCount:kitalulusCount+deallsCount])
		copy(result.LinkedIn, allJobs[kitalulusCount+deallsCount:])

		assessed, err := s.assessor.Assess(ctx, allJobs)
		if err != nil {
			if s.logger != nil {
				s.logger.Printf("jobsearch: assessment: %v", err)
			}
			return result, nil
		}

		if len(assessed) == len(allJobs) {
			copy(result.Kitalulus, assessed[:kitalulusCount])
			copy(result.Dealls, assessed[kitalulusCount:kitalulusCount+deallsCount])
			copy(result.LinkedIn, assessed[kitalulusCount+deallsCount:])
		}
	}

	return result, nil
}

func (s *JobSearchService) SearchAndDeliver(ctx context.Context, criteria domain.Criteria) error {
	result, err := s.Search(ctx, criteria)
	if err != nil {
		return err
	}

	greeting := "Selamat pagi! ☀️ Berikut update lowongan kerja terbaru hari ini:"
	if criteria.Interactive {
		greeting = "Berikut hasil pencarian lowongan kerja:"
	}

	if s.messenger == nil {
		return nil
	}

	return s.messenger.Send(ctx, domain.FormatMessage(greeting, result))
}
