# Test Matrix

Durable status is authoritative: `harness-cli query matrix --active --summary`.

| Change | Unit | Integration | Platform |
| --- | --- | --- | --- |
| Pure domain logic | required | as needed | no |
| Fiber route/contract | required | required | build |
| Finance/SQLite | required | temp-SQLite required | build |
| External provider adapter | fake required | deterministic contract test | no live traffic |
| Entrypoint/Docker | targeted | startup where safe | linux/amd64 build |

Target baseline:
`go test ./...`; `go vet ./...`; `go build ./cmd/app`;
`go build ./cmd/job-alert`; `docker build --platform linux/amd64 .`.
Record failures honestly while the target layout is under construction.
