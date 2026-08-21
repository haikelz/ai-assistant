# Runtime and Domain Ownership

Current executables live in three Go modules: `finance-api`, `job-alert`, and
`loker-api`. The accepted target consolidates them into a Go/Fiber modular
monolith with `cmd/app` and `cmd/job-alert` composition roots.

Finance owns finance rules and SQLite state. Jobsearch owns source fetching,
normalization, filtering, and delivery orchestration. AI/provider owns provider
interfaces and adapters; it does not own domain policy. Fiber handlers translate
HTTP only. Configuration and provider construction belong at composition roots.
Cross-domain calls use explicit interfaces, not package globals.
