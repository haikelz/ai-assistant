package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ai-assistant/internal/domains/jobsearch/domain"
)

const classificationInstructions = `Classify each supplied job. Return ONLY a JSON object with key "jobs" containing one result per job_id. Fields: job_id, category, seniority, relevance_score (0-100), skill_match_score (0-100), matched_skills, missing_skills, summary (one short Indonesian sentence), halal_status (halal|tidak_halal|perlu_riset), halal_reason. Relevance measures fit to the supplied criteria. For halal_status, mark tidak_halal when the company's primary business produces or sells prohibited goods/services, depends on interest-bearing (ribawi) finance, or is a bank, insurer, or online lender. Use perlu_riset when reliable business evidence is insufficient. Do not hide non-halal jobs and never fabricate evidence.`

type BatchClassifier struct {
	assessor  *AIAssessor
	batchSize int
}

func NewBatchClassifier(assessor *AIAssessor, batchSize int) *BatchClassifier {
	if batchSize < 1 || batchSize > 10 {
		batchSize = 5
	}
	return &BatchClassifier{assessor: assessor, batchSize: batchSize}
}

func (b *BatchClassifier) Classify(ctx context.Context, jobs []domain.NormalizedJob, criteria domain.Criteria) ([]domain.Classification, error) {
	result := make([]domain.Classification, 0, len(jobs))
	if len(jobs) == 0 {
		return result, nil
	}
	if b.assessor == nil || strings.TrimSpace(b.assessor.config.Model) == "" {
		return fallbackClassifications(jobs), nil
	}
	criteriaJSON := map[string]any{"positions": criteria.Positions, "skills": criteria.Skills, "locations": criteria.Locations, "max_years": criteria.MaxYears, "work_modes": criteria.WorkModes, "min_salary": criteria.MinSalary}
	for start := 0; start < len(jobs); start += b.batchSize {
		end := start + b.batchSize
		if end > len(jobs) {
			end = len(jobs)
		}
		requestJobs := make([]map[string]any, 0, end-start)
		for _, job := range jobs[start:end] {
			requestJobs = append(requestJobs, map[string]any{"job_id": job.ID, "title": job.Title, "company": job.Company, "description": job.Description, "city": job.City, "work_mode": job.WorkMode, "salary_min": job.SalaryMin, "salary_max": job.SalaryMax, "min_years_exp": job.MinYearsExp, "skills": job.Skills})
		}
		input, _ := json.Marshal(map[string]any{"criteria": criteriaJSON, "jobs": requestJobs})
		text, err := b.assessor.request(ctx, classificationInstructions, string(input))
		if err != nil {
			return append(result, fallbackClassifications(jobs[start:])...), err
		}
		var response struct {
			Jobs []struct {
				JobID           string   `json:"job_id"`
				Category        string   `json:"category"`
				Seniority       string   `json:"seniority"`
				Summary         string   `json:"summary"`
				HalalStatus     string   `json:"halal_status"`
				HalalReason     string   `json:"halal_reason"`
				RelevanceScore  float64  `json:"relevance_score"`
				SkillMatchScore float64  `json:"skill_match_score"`
				MatchedSkills   []string `json:"matched_skills"`
				MissingSkills   []string `json:"missing_skills"`
			} `json:"jobs"`
		}
		if err := json.Unmarshal([]byte(cleanJSON(text)), &response); err != nil {
			return append(result, fallbackClassifications(jobs[start:])...), fmt.Errorf("decode job classifications: %w", err)
		}
		byID := map[string]domain.Classification{}
		for _, item := range response.Jobs {
			status := item.HalalStatus
			if status != domain.HalalStatusHalal && status != domain.HalalStatusNotHalal && status != domain.HalalStatusNeedsReview {
				status = domain.HalalStatusNeedsReview
			}
			byID[item.JobID] = domain.Classification{JobID: item.JobID, Category: item.Category, Seniority: item.Seniority, Summary: strings.Join(strings.Fields(item.Summary), " "), AIRelevance: item.RelevanceScore, SkillMatch: item.SkillMatchScore, MatchedSkills: item.MatchedSkills, MissingSkills: item.MissingSkills, HalalStatus: status, HalalReason: strings.Join(strings.Fields(item.HalalReason), " ")}
		}
		for _, job := range jobs[start:end] {
			classification, ok := byID[job.ID]
			if !ok {
				classification = fallbackClassification(job)
			}
			result = append(result, classification)
		}
	}
	return result, nil
}

func fallbackClassifications(jobs []domain.NormalizedJob) []domain.Classification {
	result := make([]domain.Classification, 0, len(jobs))
	for _, job := range jobs {
		result = append(result, fallbackClassification(job))
	}
	return result
}

func fallbackClassification(job domain.NormalizedJob) domain.Classification {
	return domain.Classification{JobID: job.ID, AIRelevance: 50, HalalStatus: domain.HalalStatusNeedsReview, Summary: "Penilaian AI belum tersedia; kecocokan dihitung dari data lowongan."}
}
