# US-004: Scheduled Job-Alert Email Delivery

**Lane:** high-risk
**Status:** implemented

## Contract

When SMTP is configured, send the formatted daily job-alert result to
`MAIL_TO` in addition to Telegram and optional WhatsApp. Email applies only to
`job-alert --scheduled`; interactive `/loker` searches do not send email.

## Acceptance Criteria

- [x] `MAIL_TO` enables scheduled email while an empty value preserves existing
      Telegram and WhatsApp behavior.
- [x] SMTP host, port, username, password, sender, mailer, and encryption come
      from environment variables without logging credentials.
- [x] `MAIL_MAILER=smtp` with `MAIL_ENCRYPTION=ssl` uses implicit TLS and does
      not fall back to an unencrypted connection.
- [x] Telegram, WhatsApp, and email are attempted independently.
- [x] Email contains the same formatted job result as the other channels.
- [x] Tests use an injected fake sender and never contact a live SMTP provider.

## Risks and Rollback

SMTP credentials are secrets and must remain in Kubernetes Secret values.
Sending is a single attempt because retrying after an ambiguous SMTP result can
duplicate email. Roll back by removing `MAIL_TO`; scheduled Telegram and
WhatsApp delivery remains unchanged.

## Validation

```text
go test ./...
go test -race ./internal/domains/jobsearch/...
go vet ./...
go build ./cmd/app
go build ./cmd/job-alert
docker build --platform linux/amd64 .
```
