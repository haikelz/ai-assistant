package domain

import "time"

const (
	AlertRunRunning   = "running"
	AlertRunCompleted = "completed"
	AlertRunFailed    = "failed"
)

type AlertRun struct {
	ID, Status, ErrorMessage                  string
	StartedAt                                 time.Time
	FinishedAt                                *time.Time
	JobsFetched, JobsNew, JobsUpdated         int
	JobsFiltered, JobsClassified, JobsMatched int
}
