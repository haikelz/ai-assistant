#!/bin/sh
# Daily job-alert scheduler. Telegram updates belong to picoclaw.
set -eu

STATE_FILE="/tmp/loker-bot.state"
LAST_DAILY=$(cat "$STATE_FILE" 2>/dev/null || echo "")

echo "loker-bot: scheduler started" >&2

while true; do
  NOW_HOUR=$(TZ=Asia/Jakarta date +%H)
  NOW_DATE=$(TZ=Asia/Jakarta date +%Y-%m-%d)

  if [ "$NOW_HOUR" = "03" ] && [ "$LAST_DAILY" != "$NOW_DATE" ]; then
    echo "loker-bot: running daily job alert" >&2
    /usr/local/bin/job-alert --halal &
    echo "$NOW_DATE" > "$STATE_FILE"
    LAST_DAILY="$NOW_DATE"
  fi

  sleep 30
done
