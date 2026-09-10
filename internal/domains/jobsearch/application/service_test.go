package application

import (
	"ai-assistant/internal/domains/jobsearch/domain"
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeSource struct {
	name string
	jobs []domain.Job
	err  error
}

func (f fakeSource) Name() string { return f.name }
func (f fakeSource) Fetch(context.Context, domain.Criteria) ([]domain.Job, error) {
	return f.jobs, f.err
}

type fakeAssessor struct{ called bool }

func (a *fakeAssessor) Assess(_ context.Context, j []domain.Job) ([]domain.Job, error) {
	a.called = true
	for i := range j {
		j[i].HalalStatus = domain.HalalStatusHalal
	}
	return j, nil
}

type fakeMessenger struct{ ch chan string }

func (m fakeMessenger) Send(_ context.Context, s string) error { m.ch <- s; return nil }
func TestServiceOrchestratesAndKeepsFailedSection(t *testing.T) {
	a := &fakeAssessor{}
	m := fakeMessenger{make(chan string, 1)}
	s := NewJobSearchService([]Source{
		fakeSource{
			name: "kitalulus",
			jobs: []domain.Job{
				{
					Title:   "Go Engineer",
					Company: "A",
				},
			},
		},
		fakeSource{
			name: "dealls",
			err:  errors.New("down"),
		},
	}, a, m, nil)
	c := domain.Criteria{Skills: []string{"go"}, Halal: true, Interactive: true}
	if err := s.SearchAndDeliver(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	message := <-m.ch
	if !a.called || !strings.Contains(message, "Berikut hasil pencarian") || !strings.Contains(message, "B. Dealls") {
		t.Fatalf("called=%v message=%s", a.called, message)
	}
}

func TestServiceSendsSearchAcknowledgement(t *testing.T) {
	messenger := fakeMessenger{make(chan string, 1)}
	service := NewJobSearchService(nil, nil, messenger, nil)
	if err := service.AcknowledgeSearch(t.Context()); err != nil {
		t.Fatal(err)
	}
	if message := <-messenger.ch; message != SearchAcknowledgement {
		t.Fatalf("message=%q", message)
	}
}

func TestServiceAcknowledgementNamesEnabledLinkedInSource(t *testing.T) {
	messenger := fakeMessenger{make(chan string, 1)}
	service := NewJobSearchService([]Source{
		fakeSource{name: "kitalulus"},
		fakeSource{name: "dealls"},
		fakeSource{name: "linkedin"},
	}, nil, messenger, nil)
	if err := service.AcknowledgeSearch(t.Context()); err != nil {
		t.Fatal(err)
	}
	if message := <-messenger.ch; message != "Mencari lowongan di Kitalulus, Dealls, dan LinkedIn. Hasil akan dikirim ke chat kamu." {
		t.Fatalf("message=%q", message)
	}
}

func TestServiceKeepsPartialLinkedInResults(t *testing.T) {
	service := NewJobSearchService([]Source{
		fakeSource{
			name: "linkedin",
			jobs: []domain.Job{
				{
					Title:  "Software Engineer",
					Source: "linkedin",
				},
			},
			err: domain.ErrProviderBlocked,
		},
	}, nil, nil, nil)
	result, err := service.Search(t.Context(), domain.Criteria{Positions: []string{"Software Engineer"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.LinkedInIncluded || len(result.LinkedIn) != 1 {
		t.Fatalf("result=%#v", result)
	}
}
