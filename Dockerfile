FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -o /out/ai-assistant ./cmd/app && \
    CGO_ENABLED=0 go build -o /out/job-alert ./cmd/job-alert

FROM docker.io/sipeed/picoclaw:v0.3.1

RUN apk add --no-cache curl jq

COPY config.json /seed/config.json
COPY workspace /seed/workspace
COPY configure-ai.sh /usr/local/bin/configure-ai
COPY --from=builder /out/ai-assistant /usr/local/bin/ai-assistant
COPY --from=builder /out/job-alert /usr/local/bin/job-alert
COPY scripts/runtime/loker /usr/local/bin/loker
COPY scripts/runtime/job-alert-scheduler /usr/local/bin/job-alert-scheduler
RUN chmod +x /usr/local/bin/configure-ai /usr/local/bin/loker /usr/local/bin/job-alert-scheduler
COPY app-entrypoint.sh /app-entrypoint.sh

RUN chmod +x /app-entrypoint.sh

ENTRYPOINT ["/app-entrypoint.sh"]
