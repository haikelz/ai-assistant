#!/bin/sh
set -eu

finance-api &
finance_api_pid=$!

until curl -fsS http://127.0.0.1:8080/health >/dev/null; do
  if ! kill -0 "$finance_api_pid" 2>/dev/null; then
    exit 1
  fi
  sleep 0.1
done

exec /entrypoint.sh
