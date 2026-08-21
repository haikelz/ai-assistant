# Harness Scripts

`bootstrap-harness.sh` verifies the pinned prebuilt CLI, then initializes or
migrates the ignored local `harness.db`. Versioned migrations are in `schema/`.

```bash
scripts/bootstrap-harness.sh
scripts/bin/harness-cli --version
scripts/bin/harness-cli query matrix --active --summary
```

The CLI records workflow state; it does not validate the application. Target
application proof is `go test ./...`, `go vet ./...`, builds of `./cmd/app` and
`./cmd/job-alert`, and `docker build --platform linux/amd64 .`.
