# Tool Registry

Use repo-local `scripts/bin/harness-cli` v0.1.17 for durable Harness state.
Bootstrap before writes. Discover commands with `--help`; inspect registered
extensions with `query tools --summary`. Go commands prove application code and
Docker proves the Linux/amd64 image. `git diff --check` proves patch hygiene.

Tools are optional unless a story names them. Missing registered tools are
reported as degraded proof, never replaced by invented results. Provider APIs
are production boundaries, not validation tools.
