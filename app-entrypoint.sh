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

# Rebuild runtime Telegram config from the container secret. This avoids a
# stale PVC or an older init-container file disabling the bot.
mkdir -p /root/.picoclaw
CLEAN_BOT_TOKEN=$(printenv TELEGRAM_BOT_TOKEN | tr -d '\r\n')
if [ -n "$CLEAN_BOT_TOKEN" ]; then
  printf 'channels:\n  telegram:\n    settings:\n      token: "%s"\n' "$CLEAN_BOT_TOKEN" > /root/.picoclaw/.security.yml
  chmod 600 /root/.picoclaw/.security.yml
fi

# Keep the managed job-search instructions current even when the workspace is
# restored from an older persistent volume.
mkdir -p /root/.picoclaw/workspace/skills/job-search
cp /seed/workspace/skills/job-search/SKILL.md /root/.picoclaw/workspace/skills/job-search/SKILL.md

# Start loker-bot for daily job alert at 3 AM
loker-bot.sh &

exec /entrypoint.sh
