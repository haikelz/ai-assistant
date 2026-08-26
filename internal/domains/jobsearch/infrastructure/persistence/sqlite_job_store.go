package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type SQLiteJobStore struct{ db *sql.DB }

func NewSQLiteJobStore(db *sql.DB) *SQLiteJobStore { return &SQLiteJobStore{db: db} }

func (s *SQLiteJobStore) Initialize(ctx context.Context) error {
	statements := []string{
		`PRAGMA journal_mode=WAL`, `PRAGMA busy_timeout=5000`, `PRAGMA foreign_keys=ON`,
		`CREATE TABLE IF NOT EXISTS jobs (
            id TEXT PRIMARY KEY, source TEXT NOT NULL, external_id TEXT NOT NULL,
            canonical_url TEXT NOT NULL, title TEXT NOT NULL, normalized_title TEXT NOT NULL,
            company TEXT NOT NULL, description TEXT, city TEXT, work_mode TEXT,
            salary_min INTEGER, salary_max INTEGER, salary_currency TEXT,
            min_years_exp INTEGER, max_years_exp INTEGER, skills TEXT NOT NULL,
            employment_type TEXT, content_hash TEXT NOT NULL, source_published_at DATETIME,
            first_seen_at DATETIME NOT NULL, last_seen_at DATETIME NOT NULL,
            UNIQUE(source, external_id))`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_content_hash ON jobs(content_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_first_seen ON jobs(first_seen_at)`,
		`CREATE TABLE IF NOT EXISTS job_alert_runs (
            id TEXT PRIMARY KEY, started_at DATETIME NOT NULL, finished_at DATETIME,
            status TEXT NOT NULL, jobs_fetched INTEGER DEFAULT 0, jobs_new INTEGER DEFAULT 0,
            jobs_updated INTEGER DEFAULT 0, jobs_filtered INTEGER DEFAULT 0,
            jobs_classified INTEGER DEFAULT 0, jobs_matched INTEGER DEFAULT 0,
            error_message TEXT)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize job store: %w", err)
		}
	}
	return nil
}

func (s *SQLiteJobStore) Upsert(ctx context.Context, job domain.NormalizedJob) (domain.UpsertOutcome, error) {
	var existingHash string
	var firstSeen time.Time
	lookupErr := s.db.QueryRowContext(ctx, `SELECT content_hash, first_seen_at FROM jobs WHERE source = ? AND external_id = ?`, job.Source, job.ExternalID).Scan(&existingHash, &firstSeen)
	if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		return "", fmt.Errorf("lookup job: %w", lookupErr)
	}
	skills, err := json.Marshal(job.Skills)
	if err != nil {
		return "", fmt.Errorf("encode job skills: %w", err)
	}
	if errors.Is(lookupErr, sql.ErrNoRows) {
		_, err = s.db.ExecContext(ctx, `INSERT INTO jobs
            (id, source, external_id, canonical_url, title, normalized_title, company, description, city, work_mode, salary_min, salary_max, salary_currency, min_years_exp, max_years_exp, skills, employment_type, content_hash, source_published_at, first_seen_at, last_seen_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			job.ID, job.Source, job.ExternalID, job.CanonicalURL, job.Title, job.NormalizedTitle, job.Company, job.Description, job.City, job.WorkMode, job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.MinYearsExp, job.MaxYearsExp, skills, job.EmploymentType, job.ContentHash, job.SourcePublishedAt, job.FirstSeenAt, job.LastSeenAt)
		if err != nil {
			return "", fmt.Errorf("insert job: %w", err)
		}
		return domain.UpsertNew, nil
	}
	if existingHash == job.ContentHash {
		_, err = s.db.ExecContext(ctx, `UPDATE jobs SET last_seen_at = ? WHERE source = ? AND external_id = ?`, job.LastSeenAt, job.Source, job.ExternalID)
		if err != nil {
			return "", fmt.Errorf("touch job: %w", err)
		}
		return domain.UpsertUnchanged, nil
	}
	_, err = s.db.ExecContext(ctx, `UPDATE jobs SET canonical_url=?, title=?, normalized_title=?, company=?, description=?, city=?, work_mode=?, salary_min=?, salary_max=?, salary_currency=?, min_years_exp=?, max_years_exp=?, skills=?, employment_type=?, content_hash=?, source_published_at=?, first_seen_at=?, last_seen_at=? WHERE source=? AND external_id=?`,
		job.CanonicalURL, job.Title, job.NormalizedTitle, job.Company, job.Description, job.City, job.WorkMode, job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.MinYearsExp, job.MaxYearsExp, skills, job.EmploymentType, job.ContentHash, job.SourcePublishedAt, firstSeen, job.LastSeenAt, job.Source, job.ExternalID)
	if err != nil {
		return "", fmt.Errorf("update job: %w", err)
	}
	return domain.UpsertUpdated, nil
}

func (s *SQLiteJobStore) StartRun(ctx context.Context, run domain.AlertRun) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO job_alert_runs (id, started_at, status) VALUES (?, ?, ?)`, run.ID, run.StartedAt, run.Status)
	return err
}

func (s *SQLiteJobStore) FinishRun(ctx context.Context, run domain.AlertRun) error {
	_, err := s.db.ExecContext(ctx, `UPDATE job_alert_runs SET finished_at=?, status=?, jobs_fetched=?, jobs_new=?, jobs_updated=?, jobs_filtered=?, jobs_classified=?, jobs_matched=?, error_message=? WHERE id=?`, run.FinishedAt, run.Status, run.JobsFetched, run.JobsNew, run.JobsUpdated, run.JobsFiltered, run.JobsClassified, run.JobsMatched, run.ErrorMessage, run.ID)
	return err
}
