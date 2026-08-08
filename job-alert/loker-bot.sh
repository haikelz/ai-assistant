#!/bin/sh
# loker-bot: intercept /loker commands + daily job alert at 3 AM WIB
# Must have TELEGRAM_BOT_TOKEN and TELEGRAM_USER_ID in env
set -eu

TELEGRAM_API="https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}"
CHAT_ID="${TELEGRAM_USER_ID}"
LAST_UPDATE_ID=0
LOCK_FILE="/tmp/loker-bot.lock"
STATE_FILE="/tmp/loker-bot.state"

exec 9>"$LOCK_FILE"
if ! flock -n 9; then
	echo "loker-bot: another instance running, exit" >&2
	exit 0
fi

# Load last daily run date
LAST_DAILY=$(cat "$STATE_FILE" 2>/dev/null || echo "")

echo "loker-bot: started" >&2

while true; do
	NOW_HOUR=$(TZ=Asia/Jakarta date +%H)
	NOW_DATE=$(TZ=Asia/Jakarta date +%Y-%m-%d)

	# Daily job alert at 3 AM WIB
	if [ "$NOW_HOUR" = "03" ] && [ "$LAST_DAILY" != "$NOW_DATE" ]; then
		echo "loker-bot: running daily job alert" >&2
		/usr/local/bin/job-alert &
		echo "$NOW_DATE" > "$STATE_FILE"
		LAST_DAILY="$NOW_DATE"
	fi

	# Poll Telegram for /loker commands
	UPDATES=$(curl -sfS "${TELEGRAM_API}/getUpdates?offset=$((LAST_UPDATE_ID + 1))&timeout=10&allowed_updates=[\"message\"]" 2>/dev/null || echo '{"ok":false}')

	MAX_ID=0
	if echo "$UPDATES" | jq -e '.ok' >/dev/null 2>&1; then
		echo "$UPDATES" | jq -c '.result[]' 2>/dev/null | while read -r UPDATE; do
			UPDATE_ID=$(echo "$UPDATE" | jq -r '.update_id')
			TEXT=$(echo "$UPDATE" | jq -r '.message.text // ""')
			MSG_CHAT_ID=$(echo "$UPDATE" | jq -r '.message.chat.id // ""')

			if [ "$UPDATE_ID" -gt "$LAST_UPDATE_ID" ] 2>/dev/null; then
				# Write to temp file so parent shell can read it
				echo "$UPDATE_ID" >> /tmp/loker-bot.max_id
			fi

			case "$TEXT" in
				/loker*)
					QUERY=$(echo "$TEXT" | sed 's/^\/loker[[:space:]]*//')
					if [ -n "$QUERY" ]; then
						echo "loker-bot: /loker query=$QUERY" >&2
						curl -sfS "${TELEGRAM_API}/sendMessage" \
							-H "Content-Type: application/json" \
							-d "$(jq -n --arg c "$MSG_CHAT_ID" '{chat_id: $c, text: "Mencari lowongan di Glints & Jobstreet..."}')" \
							>/dev/null 2>&1 || true
						/usr/local/bin/loker "$QUERY" &
					else
						curl -sfS "${TELEGRAM_API}/sendMessage" \
							-H "Content-Type: application/json" \
							-d "$(jq -n --arg c "$MSG_CHAT_ID" '{chat_id: $c, text: "Format: /loker <posisi> | <skills> | <pengalaman> | <lokasi>\n\nContoh:\n/loker react developer | react,typescript | 1-3 | jakarta"}')" \
							>/dev/null 2>&1 || true
					fi
					;;
			esac
		done

		# Read max update_id from temp file
		if [ -f /tmp/loker-bot.max_id ]; then
			MAX_ID=$(tail -1 /tmp/loker-bot.max_id)
			rm -f /tmp/loker-bot.max_id
		fi
	fi

	if [ "$MAX_ID" -gt "$LAST_UPDATE_ID" ] 2>/dev/null; then
		LAST_UPDATE_ID=$MAX_ID
	fi

	sleep 2
done
