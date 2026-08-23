package application

import (
	"testing"

	"ai-assistant/internal/domains/jobsearch/domain"
)

func TestSearchPlannerIsDeterministicAndBounded(t *testing.T) {
	planner := NewSearchPlanner(3)
	queries := planner.Plan(domain.Criteria{Positions: []string{"Software Engineer", "Backend Engineer"}, Skills: []string{"Go", "TypeScript"}, Locations: []string{"Jakarta"}})
	if len(queries) != 3 || queries[0].Keyword != "Software Engineer" || queries[2].Keyword != "Software Engineer Go" {
		t.Fatalf("queries=%#v", queries)
	}
}
