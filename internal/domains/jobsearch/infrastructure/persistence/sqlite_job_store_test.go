package persistence

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
	_ "modernc.org/sqlite"
)

func TestSQLiteJobStoreDeduplicatesAndAuditsRuns(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewSQLiteJobStore(db)
	if err := store.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC)
	job := domain.NormalizedJob{ID: "dealls:1", Source: "dealls", ExternalID: "1", CanonicalURL: "https://example/1", Title: "Engineer", NormalizedTitle: "engineer", Company: "A", Skills: []string{"go"}, ContentHash: "hash-1", FirstSeenAt: now, LastSeenAt: now}
	if outcome, err := store.Upsert(t.Context(), job); err != nil || outcome != domain.UpsertNew {
		t.Fatalf("new outcome=%q err=%v", outcome, err)
	}
	job.LastSeenAt = now.Add(time.Hour)
	if outcome, err := store.Upsert(t.Context(), job); err != nil || outcome != domain.UpsertUnchanged {
		t.Fatalf("unchanged outcome=%q err=%v", outcome, err)
	}
	job.ContentHash = "hash-2"
	if outcome, err := store.Upsert(t.Context(), job); err != nil || outcome != domain.UpsertUpdated {
		t.Fatalf("updated outcome=%q err=%v", outcome, err)
	}
	run := domain.AlertRun{ID: "run-1", StartedAt: now, Status: domain.AlertRunRunning}
	if err := store.StartRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	finished := now.Add(time.Minute)
	run.FinishedAt, run.Status, run.JobsFetched, run.JobsNew = &finished, domain.AlertRunCompleted, 3, 1
	if err := store.FinishRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	var status string
	var fetched, fresh int
	if err := db.QueryRow(`SELECT status, jobs_fetched, jobs_new FROM job_alert_runs WHERE id='run-1'`).Scan(&status, &fetched, &fresh); err != nil {
		t.Fatal(err)
	}
	if status != domain.AlertRunCompleted || fetched != 3 || fresh != 1 {
		t.Fatalf("run status=%s fetched=%d new=%d", status, fetched, fresh)
	}
}
