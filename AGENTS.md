# AI Assistant Agent Instructions

<!-- HARNESS:BEGIN -->
## Harness

Classify the request before Harness operations. Answer/review/diagnosis/plan
requests are read-only: do not bootstrap or write durable state. For an explicit
change/build/fix request, run `scripts/bootstrap-harness.sh`, record intake per
`docs/FEATURE_INTAKE.md`, query
`scripts/bin/harness-cli query matrix --active --summary`, and retrieve only the
context required by `docs/CONTEXT_RULES.md`.

This repository is migrating toward a Go/Fiber modular monolith with
`cmd/app` and `cmd/job-alert` entry points. Finance, SQLite state, job-search
automation, AI/provider calls, and all external providers are high-risk. Baseline
target proof is `go test ./...`, `go vet ./...`, `go build ./cmd/app`,
`go build ./cmd/job-alert`, and a Linux/amd64 Docker build. Do not claim these
commands pass until the migration supplies the target layout.
<!-- HARNESS:END -->

Never expose `.env`, provider credentials, tokens, local databases, or finance
workbooks. Preserve existing behavior unless an accepted story/decision changes
it; use fakes and dry runs instead of live provider traffic.
