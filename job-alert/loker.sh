#!/bin/sh
# Wrapper: parse /loker input format → call job-alert
set -eu

INPUT="${1:-}"
KEYWORDS=""
SKILLS="react,node,typescript"
EXPERIENCE="1-3"
LOCATION="jakarta"

if echo "$INPUT" | grep -q '|'; then
	KEYWORDS=$(echo "$INPUT" | cut -d'|' -f1 | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
	SKILLS=$(echo "$INPUT" | cut -d'|' -f2 | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
	EXPERIENCE=$(echo "$INPUT" | cut -d'|' -f3 | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
	LOCATION=$(echo "$INPUT" | cut -d'|' -f4 | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
else
	KEYWORDS="$INPUT"
fi

[ -z "$KEYWORDS" ] && KEYWORDS="fullstack developer"
[ -z "$SKILLS" ] && SKILLS="react,node,typescript"
[ -z "$EXPERIENCE" ] && EXPERIENCE="1-3"
[ -z "$LOCATION" ] && LOCATION="jakarta"

exec /usr/local/bin/job-alert \
	--keywords "$KEYWORDS" \
	--skills "$SKILLS" \
	--experience "$EXPERIENCE" \
	--location "$LOCATION"
