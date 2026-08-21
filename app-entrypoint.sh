#!/bin/sh
set -eu

# Always rebuild model configuration from the current environment. This also
# replaces stale provider/model settings restored from the persistent volume.
configure-ai /seed/config.json /root/.picoclaw/config.json

# Start the modular monolith HTTP runtime.
ai-assistant &
app_pid=$!

until curl -fsS http://127.0.0.1:8080/health >/dev/null; do
  if ! kill -0 "$app_pid" 2>/dev/null; then
    exit 1
  fi
  sleep 0.1
done

until curl -fsS http://127.0.0.1:8081/health >/dev/null; do
  if ! kill -0 "$app_pid" 2>/dev/null; then
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

# Start the daily halal-labelled job alert at 3 AM.
job-alert-scheduler &

exec /entrypoint.sh
