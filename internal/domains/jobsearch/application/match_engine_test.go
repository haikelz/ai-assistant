package application

import (
	"math"
	"testing"

	"ai-assistant/internal/domains/jobsearch/domain"
)

func TestMatchEngineUsesDocumentedWeightsAndThreshold(t *testing.T) {
	job := domain.NormalizedJob{ID: "1", Title: "Software Engineer", Skills: []string{"go", "typescript"}, City: "Jakarta", WorkMode: domain.WorkModeRemote, SalaryMax: 15_000_000}
	criteria := domain.Criteria{Positions: []string{"Software Engineer"}, Skills: []string{"go", "typescript"}, Locations: []string{"Jakarta"}, WorkModes: []domain.WorkMode{domain.WorkModeRemote}, MinSalary: 10_000_000}
	results := NewMatchEngine(70).Rank([]domain.NormalizedJob{job}, []domain.Classification{{JobID: "1", AIRelevance: 90}}, criteria)
	if len(results) != 1 || math.Abs(results[0].FinalScore-96.5) > 0.001 {
		t.Fatalf("results=%#v", results)
	}
	if results := NewMatchEngine(97).Rank([]domain.NormalizedJob{job}, []domain.Classification{{JobID: "1", AIRelevance: 90}}, criteria); len(results) != 0 {
		t.Fatalf("threshold did not exclude result: %#v", results)
	}
}

func TestMatchEngineDefaultsThresholdToOne(t *testing.T) {
	job := domain.NormalizedJob{ID: "1", Title: "Unrelated role"}
	results := NewMatchEngine(0).Rank([]domain.NormalizedJob{job}, []domain.Classification{{JobID: "1", AIRelevance: 10}}, domain.Criteria{Positions: []string{"Software Engineer"}})
	if len(results) != 1 {
		t.Fatalf("default threshold should retain score above 1: %#v", results)
	}
}
