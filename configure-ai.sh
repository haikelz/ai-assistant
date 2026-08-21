#!/bin/sh
set -eu

provider=$(printf '%s' "${AI_PROVIDER:-sumopod}" | tr '[:upper:]' '[:lower:]')
: "${AI_MODEL:?AI_MODEL must be set}"

case "$provider" in
  sumopod)
    : "${SUMOPOD_API_KEY:?SUMOPOD_API_KEY must be set when AI_PROVIDER=sumopod}"
    PICO_PROVIDER=azure
    PICO_API_BASE=http://127.0.0.1:8080
    AI_API_KEY=$SUMOPOD_API_KEY
    ;;
  google)
    : "${GOOGLE_API_KEY:?GOOGLE_API_KEY must be set when AI_PROVIDER=google}"
    PICO_PROVIDER=gemini
    PICO_API_BASE=https://generativelanguage.googleapis.com/v1beta
    AI_API_KEY=$GOOGLE_API_KEY
    ;;
  openai)
    : "${OPENAI_API_KEY:?OPENAI_API_KEY must be set when AI_PROVIDER=openai}"
    PICO_PROVIDER=openai
    PICO_API_BASE=https://api.openai.com/v1
    AI_API_KEY=$OPENAI_API_KEY
    ;;
  *)
    echo "Unsupported AI_PROVIDER: $provider (use sumopod, google, or openai)" >&2
    exit 1
    ;;
esac

export AI_API_KEY PICO_PROVIDER PICO_API_BASE
input=${1:-/seed/config.json}
output=${2:-/root/.picoclaw/config.json}
mkdir -p "$(dirname "$output")"
temporary="$output.tmp"
trap 'rm -f "$temporary"' EXIT

jq '
  .agents.defaults.model_name = env.AI_MODEL
  | .model_list = [{
      model_name: env.AI_MODEL,
      provider: env.PICO_PROVIDER,
      model: env.AI_MODEL,
      api_base: env.PICO_API_BASE,
      api_keys: [env.AI_API_KEY]
    }]
  | .channel_list.telegram.allow_from = [env.TELEGRAM_USER_ID]
' "$input" > "$temporary"
mv "$temporary" "$output"
chmod 600 "$output"
trap - EXIT
