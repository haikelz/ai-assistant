package application

import (
	"testing"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
)

func TestNormalizeJobProducesStableCanonicalHash(t *testing.T) {
	now := time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC)
	raw := domain.RawJob{Source: "Glints", ExternalID: "abc", URL: "https://glints.test/jobs/abc?trace=secret", Title: "  Software   Engineer ", Company: "Example", Location: "Jakarta (Hybrid)", SalaryText: "Rp 12 - 16 jt", ExperienceText: "1-3 years", Skills: []string{"Go", "TypeScript", "go"}}
	job := NormalizeJob(raw, now)
	if job.ID != "glints:abc" || job.CanonicalURL != "https://glints.test/jobs/abc" || job.WorkMode != domain.WorkModeHybrid {
		t.Fatalf("job=%#v", job)
	}
	if job.SalaryMin != 12_000_000 || job.SalaryMax != 16_000_000 || job.MinYearsExp != 1 || job.MaxYearsExp != 3 {
		t.Fatalf("salary/experience=%#v", job)
	}
	if len(job.Skills) != 2 || job.ContentHash == "" {
		t.Fatalf("skills/hash=%#v", job)
	}
	changed := job
	changed.Description = "new description"
	if GenerateContentHash(changed) == job.ContentHash {
		t.Fatal("content change did not change hash")
	}
}
