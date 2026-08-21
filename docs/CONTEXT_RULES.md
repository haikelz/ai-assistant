# Context Rules

Read the minimum authoritative set:

| Phase | Required context |
| --- | --- |
| Intake | `AGENTS.md`, intake policy, active matrix summary |
| Plan | story packet, relevant product doc, accepted ADRs |
| Implement | owning package and its tests; adjacent interfaces only |
| Validate | story validation and `TEST_MATRIX.md` |
| Trace | changed paths, command results, decisions and friction |

Tiny work needs local files only. Normal work adds owning-domain contracts.
High-risk work adds all affected boundaries and rollback evidence. Retrieve more
when touching SQLite schema/state, Fiber routes/middleware, goroutines/processes,
provider calls, credentials, Docker, or when tests contradict docs. Never read
secret values or local databases merely for context.
