package application

import (
	"strings"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type SearchPlanner struct{ maxQueries int }

func NewSearchPlanner(maxQueries int) *SearchPlanner {
	if maxQueries < 1 {
		maxQueries = 5
	}
	return &SearchPlanner{maxQueries: maxQueries}
}

func (p *SearchPlanner) Plan(criteria domain.Criteria) []domain.SearchQuery {
	seen := map[string]bool{}
	var queries []domain.SearchQuery
	add := func(keyword, location string) {
		keyword, location = strings.TrimSpace(keyword), strings.TrimSpace(location)
		key := strings.ToLower(keyword + "|" + location)
		if keyword != "" && !seen[key] && len(queries) < p.maxQueries {
			seen[key] = true
			queries = append(queries, domain.SearchQuery{Keyword: keyword, Location: location})
		}
	}
	location := ""
	if len(criteria.Locations) > 0 {
		location = criteria.Locations[0]
	}
	for _, position := range criteria.Positions {
		add(position, location)
	}
	for _, position := range criteria.Positions {
		for _, skill := range criteria.Skills {
			add(position+" "+skill, location)
		}
	}
	if len(queries) == 0 {
		add("software engineer", location)
	}
	return queries
}
