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
	s := NewService([]Source{fakeSource{"kitalulus", []domain.Job{{Title: "Go Engineer", Company: "A"}}, nil}, fakeSource{"dealls", nil, errors.New("down")}}, a, m, nil)
	c := domain.Criteria{Skills: []string{"go"}, Halal: true, Interactive: true}
	if err := s.SearchAndDeliver(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	message := <-m.ch
	if !a.called || !strings.Contains(message, "Berikut hasil pencarian") || !strings.Contains(message, "B. Dealls") {
		t.Fatalf("called=%v message=%s", a.called, message)
	}
}
