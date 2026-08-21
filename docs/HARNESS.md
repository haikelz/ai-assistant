# Repository Harness

Harness turns change requests into scoped, reviewable work. For a change:
bootstrap, classify intake, query active stories, retrieve lane-specific
context, implement, validate, and record a trace. Read-only requests do none of
the durable writes.

Risk lanes are `tiny`, `normal`, and `high-risk`. Finance/SQLite, provider
traffic, job automation, public HTTP contracts, migration, and architecture are
high-risk. High-risk work requires a story packet, accepted decisions when
architecture changes, deterministic provider fakes, rollback notes, and a
detailed trace. Only `story complete` may mark implementation complete after
fresh proof. Completed work must keep its story evidence and verification
command current when follow-up changes affect the contract.
