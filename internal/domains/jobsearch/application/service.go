package application

import (
	"ai-assistant/internal/domains/jobsearch/domain"
	"context"
	"log"
	"sync"
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
				jobs = nil
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
		}
	}
	if c.Halal && s.assessor != nil {
		all := append(append([]domain.Job{}, r.Kitalulus...), r.Dealls...)
		for i := range all {
			all[i].HalalStatus = domain.HalalStatusNeedsReview
		}
		copy(r.Kitalulus, all)
		copy(r.Dealls, all[len(r.Kitalulus):])
		assessed, err := s.assessor.Assess(ctx, all)
		if err != nil {
			if s.logger != nil {
				s.logger.Printf("jobsearch: assessment: %v", err)
			}
		} else {
			copy(r.Kitalulus, assessed)
			if len(assessed) >= len(r.Kitalulus) {
				copy(r.Dealls, assessed[len(r.Kitalulus):])
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
