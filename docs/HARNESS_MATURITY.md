# Harness Maturity

- **H0 bare:** no repository policy.
- **H1 scaffolded:** instructions, intake, architecture, templates.
- **H2 durable:** CLI/database, stories, decisions, traces.
- **H3 observable:** scored traces, audits, improvement loop.
- **H4 automated:** proof gates reliably execute the target matrix.
- **H5 adaptive:** measured improvements safely evolve policy.

This repository is at H4: durable tooling and policy are installed, story proof
runs the target Go checks, and the Linux/amd64 image is built as part of the
verification gate. Traces and audits remain the feedback inputs for future H5
adaptation.
