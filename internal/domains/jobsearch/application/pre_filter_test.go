package application

import (
	"testing"

	"ai-assistant/internal/domains/jobsearch/domain"
)

func TestPreFilterAppliesCheapRulesBeforeAI(t *testing.T) {
	jobs := []domain.NormalizedJob{
		{ID: "good", Title: "Software Engineer", City: "Jakarta Selatan", WorkMode: domain.WorkModeRemote, SalaryMax: 15_000_000, MinYearsExp: 2},
		{ID: "sales", Title: "Software Sales", City: "Jakarta", WorkMode: domain.WorkModeRemote, SalaryMax: 20_000_000},
		{ID: "senior", Title: "Software Engineer", City: "Jakarta", WorkMode: domain.WorkModeRemote, SalaryMax: 20_000_000, MinYearsExp: 5},
		{ID: "onsite", Title: "Software Engineer", City: "Jakarta", WorkMode: domain.WorkModeOnsite, SalaryMax: 20_000_000},
	}
	criteria := domain.Criteria{Positions: []string{"Software Engineer"}, Locations: []string{"Jakarta"}, ExcludeKeywords: []string{"sales"}, WorkModes: []domain.WorkMode{domain.WorkModeRemote}, StrictWorkMode: true, MaxYears: 3, MinSalary: 10_000_000}
	filtered := (PreFilter{}).Apply(jobs, criteria)
	if len(filtered) != 1 || filtered[0].ID != "good" {
		t.Fatalf("filtered=%#v", filtered)
	}
}

func TestPreFilterRetainsUnknownLocationForLaterAssessment(t *testing.T) {
	job := domain.NormalizedJob{Title: "Software Engineer"}
	filtered := (PreFilter{}).Apply([]domain.NormalizedJob{job}, domain.Criteria{Positions: []string{"Software Engineer"}, Locations: []string{"Jakarta"}})
	if len(filtered) != 1 {
		t.Fatalf("unknown location must not be treated as a known mismatch: %#v", filtered)
	}
}
