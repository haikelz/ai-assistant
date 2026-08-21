# Documentation Map

- `HARNESS.md`, `FEATURE_INTAKE.md`, `CONTEXT_RULES.md`: operating workflow.
- `ARCHITECTURE.md` and `product/`: system boundaries and runtime facts.
- `TEST_MATRIX.md`, `TRACE_SPEC.md`, `TOOL_REGISTRY.md`: proof and tooling.
- `decisions/`: accepted architectural choices.
- `stories/`: implementation packets; `templates/`: reusable formats.
- `HARNESS_*` and `IMPROVEMENT_PROTOCOL.md`: harness health and evolution.

Markdown defines policy and contracts. The ignored `harness.db` contains local
operational state and is queried through `scripts/bin/harness-cli`.
