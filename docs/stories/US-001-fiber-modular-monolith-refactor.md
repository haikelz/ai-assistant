# US-001: Fiber Modular Monolith Refactor

**Lane:** high-risk  
**Status:** implemented

## Contract

Consolidate the current Go services into the ADR-0002 target while preserving
finance, job-search, and AI/provider behavior. This packet does not authorize
Harness tooling owners to modify application code.

## Acceptance Criteria

- [x] One root Go module builds `cmd/app` and `cmd/job-alert`.
- [x] `cmd/app` uses Fiber and exposes preserved finance/loker contracts.
- [x] Finance SQLite mutations and reads pass tests using isolated temporary DBs.
- [x] AI, job boards, messaging, and spreadsheet providers are injected and
      covered by deterministic fakes; baseline proof sends no live traffic.
- [x] Existing service behavior is migrated or explicitly retired by a later ADR.
- [x] All baseline commands pass, including Linux/amd64 Docker build.

## Execution and Risk

Inventory contracts, establish domain interfaces, migrate one boundary at a
time, keep commits reversible, and retain old entry points until replacement
proof exists. Back up user finance state before any production migration. Stop
on ambiguous routes, SQLite migration/data-loss risk, or provider side effects.

## Validation Commands

```bash
go test ./...
go vet ./...
go build ./cmd/app
go build ./cmd/job-alert
docker build --platform linux/amd64 .
```

Focused Fiber contracts, isolated SQLite tests, provider fakes, race tests, a
two-listener runtime smoke test, and a Linux/amd64 container health check provide
the implementation evidence. Durable proof is recorded by Harness story
`US-001`.
