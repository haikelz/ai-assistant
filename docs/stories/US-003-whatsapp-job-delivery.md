# US-003: WhatsApp Job Delivery

**Lane:** high-risk
**Status:** implemented

## Contract

Deliver `/loker` and scheduled job-alert results to WhatsApp in addition to
Telegram through Whatsmeow. Pair an unregistered device by printing a transient
terminal QR to the application pod logs. Persist the device session on the
existing PicoClaw PVC and reconnect without another QR after pod restarts.

## Acceptance Criteria

- [x] WhatsApp is optional; an unset recipient preserves Telegram-only behavior.
- [x] An unpaired configured client emits QR pairing output to stdout before
      connecting, while a paired client reconnects from the persistent store.
- [x] Session state is stored under `/root/.picoclaw` with no credentials or QR
      payload exposed through HTTP or Telegram.
- [x] `/loker` and scheduled alerts fan out to Telegram and WhatsApp; one channel
      failure does not suppress the other.
- [x] WhatsApp messages are split within a conservative text limit and use a
      normalized Indonesian/international phone JID.
- [x] Tests use fakes and temporary stores; no live WhatsApp traffic occurs.

## Scope / Non-goals

This story supports one paired account and one configured recipient. It does not
receive WhatsApp commands, provide a web QR page, manage multiple accounts, or
implement email delivery.

## Risks and Rollback

QR codes grant temporary account access and must appear only while pairing.
WhatsApp Web is unofficial automation and may disconnect or restrict the linked
account. Keep Telegram independent, make WhatsApp opt-in, persist only device
state on the PVC, and avoid blind message retries. Roll back by removing
`WHATSAPP_RECIPIENT`; Telegram and job search continue unchanged.

## Validation

```text
go test ./...
go test -race ./internal/domains/jobsearch/...
go vet ./...
go build ./cmd/app
go build ./cmd/job-alert
docker build --platform linux/amd64 .
```
