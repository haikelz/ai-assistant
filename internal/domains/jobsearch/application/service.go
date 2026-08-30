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
type Service struct {
	sources   []Source
	assessor  Assessor
	messenger Messenger
	logger    *log.Logger
}

func NewService(sources []Source, assessor Assessor, messenger Messenger, logger *log.Logger) *Service {
	return &Service{append([]Source(nil), sources...), assessor, messenger, logger}
}

const SearchAcknowledgement = "Mencari lowongan di Kitalulus dan Dealls. Hasil akan dikirim ke chat kamu."

func (s *Service) AcknowledgeSearch(ctx context.Context) error {
	if s.messenger == nil {
		return nil
	}
	return s.messenger.Send(ctx, s.searchAcknowledgement())
}

func (s *Service) searchAcknowledgement() string {
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

func (s *Service) Search(ctx context.Context, c domain.Criteria) (domain.Result, error) {
	type fetched struct {
		name string
		jobs []domain.Job
	}
	ch := make(chan fetched, len(s.sources))
	var wg sync.WaitGroup
	for _, src := range s.sources {
		wg.Add(1)
		go func(src Source) {
			defer wg.Done()
			jobs, err := src.Fetch(ctx, c)
			if err != nil {
				if s.logger != nil {
					s.logger.Printf("jobsearch: %s fetch: %v", src.Name(), err)
				}
				if len(jobs) == 0 {
					jobs = nil
				}
			}
			ch <- fetched{src.Name(), domain.FilterAndSort(jobs, c, 20)}
		}(src)
	}
	wg.Wait()
	close(ch)
	var r domain.Result
	for f := range ch {
		switch f.name {
		case "kitalulus":
			r.Kitalulus = f.jobs
		case "dealls":
			r.Dealls = f.jobs
		case "linkedin":
			r.LinkedIn = f.jobs
			r.LinkedInIncluded = true
		}
	}
	if c.Halal && s.assessor != nil {
		kitalulusCount, deallsCount := len(r.Kitalulus), len(r.Dealls)
		all := append(append(append([]domain.Job{}, r.Kitalulus...), r.Dealls...), r.LinkedIn...)
		for i := range all {
			all[i].HalalStatus = domain.HalalStatusNeedsReview
		}
		copy(r.Kitalulus, all[:kitalulusCount])
		copy(r.Dealls, all[kitalulusCount:kitalulusCount+deallsCount])
		copy(r.LinkedIn, all[kitalulusCount+deallsCount:])
		assessed, err := s.assessor.Assess(ctx, all)
		if err != nil {
			if s.logger != nil {
				s.logger.Printf("jobsearch: assessment: %v", err)
			}
		} else {
			if len(assessed) == len(all) {
				copy(r.Kitalulus, assessed[:kitalulusCount])
				copy(r.Dealls, assessed[kitalulusCount:kitalulusCount+deallsCount])
				copy(r.LinkedIn, assessed[kitalulusCount+deallsCount:])
			}
		}
	}
	return r, nil
}
func (s *Service) SearchAndDeliver(ctx context.Context, c domain.Criteria) error {
	r, err := s.Search(ctx, c)
	if err != nil {
		return err
	}
	g := "Selamat pagi! ☀️ Berikut update lowongan kerja terbaru hari ini:"
	if c.Interactive {
		g = "Berikut hasil pencarian lowongan kerja:"
	}
	if s.messenger == nil {
		return nil
	}
	return s.messenger.Send(ctx, domain.FormatMessage(g, r))
}
