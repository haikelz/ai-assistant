# ADR-0001: Adapt Repository Harness

**Status:** accepted  
**Date:** 2026-08-21

## Context

Repository-wide refactoring needs durable intake, risk, decisions, stories, and
proof without importing another product's rules.

## Decision

Install pinned Harness CLI v0.1.17, migrations 001-013, concise project policy,
and an ignored fresh local `harness.db`. Harness docs/tooling are independently
owned; application code and refactor completion remain outside this workstream.

## Consequences

Change requests gain queryable local state. The binary is platform-specific and
must match its release pin. No operational records are copied from the reference.

## Verification

Bootstrap, version check, active matrix query, and `git diff --check`.
