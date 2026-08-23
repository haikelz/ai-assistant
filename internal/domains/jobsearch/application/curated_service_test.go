package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type curatedIngestor struct{ result PipelineResult }

func (i curatedIngestor) Run(context.Context, domain.Criteria, bool) (PipelineResult, error) {
	return i.result, nil
}

type curatedClassifier struct{ err error }

func (c curatedClassifier) Classify(_ context.Context, jobs []domain.NormalizedJob, _ domain.Criteria) ([]domain.Classification, error) {
	classifications := make([]domain.Classification, len(jobs))
	for index, job := range jobs {
		classifications[index] = domain.Classification{JobID: job.ID, AIRelevance: 100, HalalStatus: domain.HalalStatusHalal}
	}
	return classifications, c.err
}

type curatedMatcher struct{}

func (curatedMatcher) Rank(jobs []domain.NormalizedJob, classifications []domain.Classification, _ domain.Criteria) []domain.MatchResult {
	if len(jobs) == 0 {
		return nil
	}
	return []domain.MatchResult{{Job: jobs[0], Classification: classifications[0], FinalScore: 90}}
}

type curatedRunStore struct {
	started, finished bool
	run               domain.AlertRun
}

func (s *curatedRunStore) StartRun(context.Context, domain.AlertRun) error {
	s.started = true
	return nil
}
func (s *curatedRunStore) FinishRun(_ context.Context, run domain.AlertRun) error {
	s.finished, s.run = true, run
	return nil
}

type curatedMessenger struct{ err error }

func (m curatedMessenger) Send(context.Context, string) error { return m.err }

func TestCuratedServiceAuditsAndContinuesWithClassifierFallback(t *testing.T) {
	job := domain.NormalizedJob{ID: "1", Title: "Software Engineer", Company: "PT Maju", CanonicalURL: "https://example.test/job"}
	store := &curatedRunStore{}
	service := NewCuratedService(curatedIngestor{PipelineResult{Jobs: []domain.NormalizedJob{job}, Fetched: 4, New: 1, Updated: 1}}, curatedClassifier{err: errors.New("AI unavailable")}, curatedMatcher{}, store, curatedMessenger{}, nil)
	message, run, err := service.Run(t.Context(), domain.Criteria{}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if !store.started || !store.finished || run.Status != domain.AlertRunCompleted || run.JobsFetched != 4 || run.JobsClassified != 1 || run.JobsMatched != 1 || !strings.Contains(run.ErrorMessage, "AI unavailable") {
		t.Fatalf("run=%#v store=%#v", run, store)
	}
	if !strings.Contains(message, "PT Maju (Halal)") {
		t.Fatalf("message=%q", message)
	}
}

func TestCuratedServiceRecordsDeliveryFailure(t *testing.T) {
	store := &curatedRunStore{}
	service := NewCuratedService(curatedIngestor{}, curatedClassifier{}, curatedMatcher{}, store, curatedMessenger{err: errors.New("down")}, nil)
	_, run, err := service.Run(t.Context(), domain.Criteria{}, true, true)
	if err == nil || run.Status != domain.AlertRunFailed || store.run.Status != domain.AlertRunFailed {
		t.Fatalf("err=%v run=%#v stored=%#v", err, run, store.run)
	}
}
