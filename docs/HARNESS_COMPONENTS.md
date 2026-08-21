# Harness Components

| Responsibility | Surface | Status |
| --- | --- | --- |
| Task/context policy | `AGENTS.md`, intake/context docs | covered |
| Project memory | product docs, ADRs, stories | covered |
| Durable task state | CLI + ignored SQLite database | covered |
| Verification | story commands and test matrix | covered |
| Observability | traces and audit | partial; manual capture |
| Tool access | pinned prebuilt CLI and registry | covered |
| Improvement | backlog/proposal protocol | covered |
| Permission enforcement | instructions | partial; advisory |

The Harness owns documentation and engineering-workflow state; application
boundaries and proof remain in the Go module and its deterministic tests.
