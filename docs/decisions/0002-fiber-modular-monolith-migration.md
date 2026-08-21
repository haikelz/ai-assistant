# ADR-0002: Migrate to a Fiber Modular Monolith

**Status:** accepted  
**Date:** 2026-08-21

## Context

Finance, job-alert, and loker HTTP behavior currently span separate Go modules
and `net/http` entry points, making shared provider boundaries and repository-wide
proof inconsistent.

## Decision

Incrementally converge on one Go module with Fiber at `cmd/app`, a separate
`cmd/job-alert`, and finance, jobsearch, and AI/provider domain packages.
Preserve observable contracts during migration. Keep SQLite and provider
adapters behind injected interfaces; use temporary databases and fake providers.

## Consequences

The migration is high-risk and remains incomplete until all target proof passes.
No auth, PostgreSQL, or Redis capability is introduced by this decision.

## Verification

Run the baseline commands in `docs/TEST_MATRIX.md`, including Linux/amd64 Docker
build, plus focused SQLite and provider-contract tests.
