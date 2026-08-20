#!/bin/sh
# Wrapper: parse /loker input format → call job-alert
set -eu

INPUT="${1:-}"
KEYWORDS=""
SKILLS=""
EXPERIENCE=""
LOCATION=""

if echo "$INPUT" | grep -q '|'; then
	KEYWORDS=$(echo "$INPUT" | cut -d'|' -f1 | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
	SKILLS=$(echo "$INPUT" | cut -d'|' -f2 | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
	EXPERIENCE=$(echo "$INPUT" | cut -d'|' -f3 | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
	LOCATION=$(echo "$INPUT" | cut -d'|' -f4 | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
else
	KEYWORDS="$INPUT"
fi


exec /usr/local/bin/job-alert \
	--keywords "$KEYWORDS" \
	--skills "$SKILLS" \
	--experience "$EXPERIENCE" \
	--location "$LOCATION"
