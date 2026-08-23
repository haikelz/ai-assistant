package application

import (
	"strings"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type PreFilter struct{}

func (PreFilter) Apply(jobs []domain.NormalizedJob, criteria domain.Criteria) []domain.NormalizedJob {
	filtered := make([]domain.NormalizedJob, 0, len(jobs))
	for _, job := range jobs {
		title := strings.ToLower(job.Title)
		if containsAny(title, criteria.ExcludeKeywords) || !matchesAny(title, criteria.Positions, true) {
			continue
		}
		if criteria.MaxYears > 0 && job.MinYearsExp > criteria.MaxYears {
			continue
		}
		if criteria.MinSalary > 0 && job.SalaryMax > 0 && job.SalaryMax < criteria.MinSalary {
			continue
		}
		if len(criteria.Locations) > 0 && !matchesAny(strings.ToLower(job.City), criteria.Locations, false) {
			continue
		}
		if criteria.StrictWorkMode && len(criteria.WorkModes) > 0 && job.WorkMode != domain.WorkModeUnknown && !workModeAllowed(job.WorkMode, criteria.WorkModes) {
			continue
		}
		filtered = append(filtered, job)
	}
	return filtered
}

func containsAny(text string, values []string) bool {
	for _, value := range values {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" && strings.Contains(text, value) {
			return true
		}
	}
	return false
}

func matchesAny(text string, values []string, emptyMatches bool) bool {
	if len(values) == 0 {
		return emptyMatches
	}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" && (strings.Contains(text, value) || containsWord(text, value)) {
			return true
		}
	}
	return false
}

func containsWord(text, value string) bool {
	for _, word := range strings.Fields(value) {
		if len(word) > 2 && strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func workModeAllowed(mode domain.WorkMode, allowed []domain.WorkMode) bool {
	for _, candidate := range allowed {
		if mode == candidate {
			return true
		}
	}
	return false
}
