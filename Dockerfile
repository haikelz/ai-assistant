FROM golang:1.25-alpine AS finance-builder

WORKDIR /src

COPY finance-api/go.mod finance-api/go.sum ./
RUN go mod download

COPY finance-api ./
RUN CGO_ENABLED=0 go build -o /out/finance-api .

FROM golang:1.25-alpine AS job-alert-builder

WORKDIR /src

COPY job-alert/go.mod ./
RUN go mod download

COPY job-alert ./
RUN CGO_ENABLED=0 go build -o /out/job-alert .

FROM golang:1.25-alpine AS loker-api-builder

WORKDIR /src

COPY loker-api/go.mod ./
RUN go mod download

COPY loker-api ./
RUN CGO_ENABLED=0 go build -o /out/loker-api .

FROM docker.io/sipeed/picoclaw:v0.3.1

RUN apk add --no-cache curl jq

COPY --from=finance-builder /out/finance-api /usr/local/bin/finance-api
COPY --from=job-alert-builder /out/job-alert /usr/local/bin/job-alert
COPY --from=loker-api-builder /out/loker-api /usr/local/bin/loker-api
COPY job-alert/loker.sh /usr/local/bin/loker
COPY job-alert/loker-bot.sh /usr/local/bin/loker-bot.sh
RUN chmod +x /usr/local/bin/loker /usr/local/bin/loker-bot.sh
COPY app-entrypoint.sh /app-entrypoint.sh

RUN chmod +x /app-entrypoint.sh

ENTRYPOINT ["/app-entrypoint.sh"]
