# Trace Specification

Every changed task records outcome, linked intake/story, agent, actions, files
read/changed, errors, and friction. Tiny traces may be minimal; normal traces
are standard; high-risk traces are detailed and also record decisions,
duration/token estimate (or why unavailable), skipped proof, provider isolation,
and rollback concerns. Outcomes are `completed`, `partial`, `blocked`, or
`failed`. A completed harness-install trace does not imply the application
refactor story is implemented.
