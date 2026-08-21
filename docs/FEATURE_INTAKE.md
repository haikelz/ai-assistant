# Feature Intake

Choose an input type: `maintenance` for repository/refactor work,
`new_initiative` for a new product outcome, `bug`, or `harness_improvement`.
Then choose a lane:

- **tiny:** isolated docs/copy with no contract or workflow effect.
- **normal:** bounded implementation with established boundaries.
- **high-risk:** architecture, finance/SQLite, migration, external provider,
  automation, or public HTTP behavior.

Record change requests with `harness-cli intake --type ... --summary ...
--lane ... --flags ... --docs ... --story ...`. High-risk intake must identify
acceptance criteria, rollback/containment, provider isolation, and validation.
