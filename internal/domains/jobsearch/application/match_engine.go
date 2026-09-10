package application

import (
	"sort"
	"strings"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type MatchEngine struct {
	threshold float64
}

func NewMatchEngine(threshold float64) *MatchEngine {
	if threshold <= 0 {
		threshold = 1
	}
	return &MatchEngine{threshold: threshold}
}

func (e *MatchEngine) Rank(
	jobs []domain.NormalizedJob,
	classifications []domain.Classification,
	criteria domain.Criteria,
) []domain.MatchResult {
	byID := make(map[string]domain.Classification, len(classifications))
	for _, classification := range classifications {
		byID[classification.JobID] = classification
	}
	var results []domain.MatchResult
	for _, job := range jobs {
		classification := byID[job.ID]
		if classification.JobID == "" {
			classification = domain.Classification{
				JobID:       job.ID,
				AIRelevance: 50,
				HalalStatus: domain.HalalStatusNeedsReview,
			}
		}
		result := domain.MatchResult{
			Job:            job,
			Classification: classification,
		}
		result.SkillScore = skillScore(job, criteria.Skills)
		result.RoleScore = roleScore(job, criteria.Positions)
		result.WorkModeScore = workModeScore(job, criteria)
		result.SalaryScore = salaryScore(job, criteria.MinSalary)
		result.LocationScore = locationScore(job, criteria.Locations)
		result.FinalScore =
			0.35*clamp(classification.AIRelevance) +
				0.30*result.SkillScore +
				0.15*result.RoleScore +
				0.10*result.WorkModeScore +
				0.05*result.SalaryScore +
				0.05*result.LocationScore
		threshold := e.threshold
		if criteria.MinMatchScore > 0 {
			threshold = criteria.MinMatchScore
		}
		if result.FinalScore >= threshold {
			results = append(results, result)
		}
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].FinalScore > results[j].FinalScore })
	return results
}

func skillScore(job domain.NormalizedJob, wanted []string) float64 {
	if len(wanted) == 0 {
		return 100
	}
	text := strings.ToLower(job.Title + " " + strings.Join(job.Skills, " ") + " " + job.Description)
	matched := 0
	for _, skill := range wanted {
		if strings.Contains(text, strings.ToLower(strings.TrimSpace(skill))) {
			matched++
		}
	}
	return 100 * float64(matched) / float64(len(wanted))
}

func roleScore(job domain.NormalizedJob, positions []string) float64 {
	if len(positions) == 0 {
		return 100
	}
	title := strings.ToLower(job.Title)
	for _, position := range positions {
		if strings.Contains(title, strings.ToLower(strings.TrimSpace(position))) {
			return 100
		}
	}
	if matchesAny(title, positions, false) {
		return 75
	}
	return 0
}

func workModeScore(job domain.NormalizedJob, criteria domain.Criteria) float64 {
	if len(criteria.WorkModes) == 0 {
		return 100
	}
	if job.WorkMode == domain.WorkModeUnknown {
		return 50
	}
	if workModeAllowed(job.WorkMode, criteria.WorkModes) {
		return 100
	}
	return 0
}

func salaryScore(job domain.NormalizedJob, minimum int64) float64 {
	if minimum == 0 {
		return 100
	}
	if job.SalaryMax == 0 {
		return 50
	}
	if job.SalaryMax >= minimum {
		return 100
	}
	return clamp(100 * float64(job.SalaryMax) / float64(minimum))
}

func locationScore(job domain.NormalizedJob, locations []string) float64 {
	if len(locations) == 0 {
		return 100
	}
	if matchesAny(strings.ToLower(job.City), locations, false) {
		return 100
	}
	return 0
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
