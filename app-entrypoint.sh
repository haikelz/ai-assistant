#!/bin/sh
set -eu

# Start finance-api
finance-api &
finance_api_pid=$!

until curl -fsS http://127.0.0.1:8080/health >/dev/null; do
  if ! kill -0 "$finance_api_pid" 2>/dev/null; then
    exit 1
  fi
  sleep 0.1
done

# Start loker-api (handles /loker commands via curl from AI)
loker-api &
loker_api_pid=$!

until curl -fsS http://127.0.0.1:8081/health >/dev/null; do
  if ! kill -0 "$loker_api_pid" 2>/dev/null; then
    exit 1
  fi
  sleep 0.1
done

# Start loker-bot for daily job alert at 3 AM
loker-bot.sh &

exec /entrypoint.sh
