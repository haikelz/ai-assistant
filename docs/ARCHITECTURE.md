# Architecture Contract

The repository is one Go module and a modular monolith. `cmd/app` hosts the
Fiber HTTP runtime; `cmd/job-alert` hosts scheduled/on-demand job search. Domain
packages own finance, jobsearch, and AI/provider behavior. Interfaces point
inward; SQLite and HTTP provider adapters sit at the edge.

The process keeps compatibility listeners on 8080 (finance and Responses proxy)
and 8081 (job-search command), but both are composed by `internal/app` and share
one lifecycle. Keep finance mutations consistent and test SQLite with isolated
temporary databases. Inject provider clients; tests must not call live AI,
job-board, Telegram, Google, or other external services. No auth, PostgreSQL, or
Redis architecture is assumed.
