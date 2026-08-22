# US-002: Configurable Job Alert

**Lane:** high-risk  
**Status:** implemented

## Contract

Allow the Telegram user to update the daily job-alert search criteria through
`/job-alert` using the same pipe-delimited criteria as `/loker`. Persist the
configuration on the existing PicoClaw volume and make the 03:00 Asia/Jakarta
scheduler use the latest saved value. Daily results keep halal labeling enabled.

## Acceptance Criteria

- [x] `POST /job-alert` validates and persists search criteria.
- [x] `GET /job-alert` returns the current configuration.
- [x] Configuration survives process and pod restarts on the mounted volume.
- [x] The scheduled CLI run uses saved criteria and falls back safely when no
      configuration exists.
- [x] `/loker` behavior remains unchanged and the daily run keeps halal labels.
- [x] Provider boundaries are tested with fakes and no live traffic.

## Scope / Non-goals

This story changes search criteria only. It does not change the 03:00 schedule,
add multiple schedules, or add a disable command.

## Risks and Rollback

Malformed or partially written state could broaden searches. Use an atomic JSON
file, validate on write and read, and fall back to the existing all-listings
daily search with halal labels when no file exists. Roll back by restoring the
scheduler's direct `job-alert --halal` invocation and removing the settings
routes; no finance data or provider credentials are stored in this file.

## Validation

```text
go test ./...
go test -race ./internal/domains/jobsearch/...
go vet ./...
go build ./cmd/app
go build ./cmd/job-alert
docker build --platform linux/amd64 .
```
