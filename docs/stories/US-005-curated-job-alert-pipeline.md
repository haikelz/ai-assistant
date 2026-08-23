# US-005: Curated Job-Alert Pipeline

**Lane:** high-risk
**Status:** implemented

## Contract

Build a scheduled-first job pipeline with bounded search planning,
Glints/Dealls/Kitalulus ingestion, normalized entities, SQLite content-hash
deduplication, deterministic filtering, batched AI classification, weighted
hybrid scoring, categorized digest delivery, and alert-run auditing.

Interactive `/loker` searches continue to show current matching jobs and must
not hide listings merely because a scheduled run saw them earlier. The existing
in-pod scheduler remains the owner of scheduled execution.

## Acceptance Criteria

- [x] Providers return source-independent raw jobs and fail independently.
- [x] Glints reads only public server-rendered listings, is rate-limited and
      bounded, and stops on access-control or challenge responses.
- [x] Normalization creates stable IDs and deterministic SHA-256 content hashes.
- [x] SQLite records new, changed, and unchanged jobs with first/last-seen times
      and persists alert-run counters and status.
- [x] Scheduled runs skip unchanged jobs before AI calls; interactive searches
      remain unaffected by historical deduplication.
- [x] Search planning caps query variations and provider concurrency/timeouts.
- [x] Deterministic pre-filtering applies role, exclusion, work-mode,
      experience, salary, and location rules before AI classification.
- [x] AI classification is batched and produces relevance, skill, seniority,
      summary, and halal fields with conservative fallback values.
- [x] Hybrid scoring uses the documented 35/30/15/10/5/5 weights and a default
      threshold of 70.
- [x] Telegram, WhatsApp, and email receive one categorized digest.
- [x] Tests use temporary SQLite, fixtures, and fake AI/provider clients only.

## Risks and Rollback

Provider layouts and access policies can change. Provider failures do not fail
other providers, and access-control responses are not retried or bypassed.
SQLite uses WAL and a busy timeout on the existing PVC. Disable Glints with
`GLINTS_ENABLED=false`; disable the new scheduled pipeline with
`JOB_ALERT_PIPELINE_ENABLED=false` during migration.
