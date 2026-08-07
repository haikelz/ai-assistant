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

FROM docker.io/sipeed/picoclaw:v0.3.1

RUN apk add --no-cache curl jq

COPY config.json /seed/config.json
COPY --from=finance-builder /out/finance-api /usr/local/bin/finance-api
COPY --from=job-alert-builder /out/job-alert /usr/local/bin/job-alert
COPY app-entrypoint.sh /app-entrypoint.sh

RUN chmod +x /app-entrypoint.sh

ENTRYPOINT ["/app-entrypoint.sh"]
