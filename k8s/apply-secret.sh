#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname "$script_dir")
namespace=${KUBERNETES_NAMESPACE:-default}

if [ ! -f "$project_dir/.env" ]; then
  echo "Missing $project_dir/.env. Copy .env.example first." >&2
  exit 1
fi

set -a
. "$project_dir/.env"
set +a

: "${AI_PROVIDER:?AI_PROVIDER must be set}"
: "${AI_MODEL:?AI_MODEL must be set}"
: "${TELEGRAM_BOT_TOKEN:?TELEGRAM_BOT_TOKEN must be set}"
: "${TELEGRAM_USER_ID:?TELEGRAM_USER_ID must be set}"

case "$(printf '%s' "$AI_PROVIDER" | tr '[:upper:]' '[:lower:]')" in
  sumopod) : "${SUMOPOD_API_KEY:?SUMOPOD_API_KEY must be set when AI_PROVIDER=sumopod}" ;;
  google) : "${GOOGLE_API_KEY:?GOOGLE_API_KEY must be set when AI_PROVIDER=google}" ;;
  openai) : "${OPENAI_API_KEY:?OPENAI_API_KEY must be set when AI_PROVIDER=openai}" ;;
  *) echo "AI_PROVIDER must be sumopod, google, or openai" >&2; exit 1 ;;
esac

if [ -n "${MAIL_TO:-}" ]; then
  : "${MAIL_MAILER:?MAIL_MAILER must be set when scheduled email is enabled}"
  : "${MAIL_USERNAME:?MAIL_USERNAME must be set when scheduled email is enabled}"
  : "${MAIL_PASSWORD:?MAIL_PASSWORD must be set when scheduled email is enabled}"
  : "${MAIL_HOST:?MAIL_HOST must be set when scheduled email is enabled}"
  : "${MAIL_PORT:?MAIL_PORT must be set when scheduled email is enabled}"
  : "${MAIL_ENCRYPTION:?MAIL_ENCRYPTION must be set when scheduled email is enabled}"
  : "${MAIL_FROM:?MAIL_FROM must be set when scheduled email is enabled}"
  [ "$(printf '%s' "$MAIL_MAILER" | tr '[:upper:]' '[:lower:]')" = "smtp" ] || { echo "MAIL_MAILER must be smtp" >&2; exit 1; }
  [ "$(printf '%s' "$MAIL_ENCRYPTION" | tr '[:upper:]' '[:lower:]')" = "ssl" ] || { echo "MAIL_ENCRYPTION must be ssl" >&2; exit 1; }
fi

set -- kubectl create secret generic ai-assistant-env \
  --namespace "$namespace" \
  --dry-run=client \
  --output yaml \
  --from-literal=AI_PROVIDER="$AI_PROVIDER" \
  --from-literal=AI_MODEL="$AI_MODEL" \
  --from-literal=SUMOPOD_API_KEY="${SUMOPOD_API_KEY:-}" \
  --from-literal=GOOGLE_API_KEY="${GOOGLE_API_KEY:-}" \
  --from-literal=OPENAI_API_KEY="${OPENAI_API_KEY:-}" \
  --from-literal=TELEGRAM_BOT_TOKEN="$TELEGRAM_BOT_TOKEN" \
  --from-literal=TELEGRAM_USER_ID="$TELEGRAM_USER_ID" \
  --from-literal=WHATSAPP_RECIPIENT="${WHATSAPP_RECIPIENT:-}" \
  --from-literal=MAIL_MAILER="${MAIL_MAILER:-}" \
  --from-literal=MAIL_USERNAME="${MAIL_USERNAME:-}" \
  --from-literal=MAIL_PASSWORD="${MAIL_PASSWORD:-}" \
  --from-literal=MAIL_HOST="${MAIL_HOST:-}" \
  --from-literal=MAIL_PORT="${MAIL_PORT:-}" \
  --from-literal=MAIL_ENCRYPTION="${MAIL_ENCRYPTION:-}" \
  --from-literal=MAIL_FROM="${MAIL_FROM:-}" \
  --from-literal=MAIL_TO="${MAIL_TO:-}"

if [ -n "${GOOGLE_SHEETS_SPREADSHEET_ID:-}" ] || [ -n "${GOOGLE_SERVICE_ACCOUNT_JSON_BASE64:-}" ]; then
  : "${GOOGLE_SHEETS_SPREADSHEET_ID:?GOOGLE_SHEETS_SPREADSHEET_ID must be set when Google Sheets is enabled}"
  : "${GOOGLE_SERVICE_ACCOUNT_JSON_BASE64:?GOOGLE_SERVICE_ACCOUNT_JSON_BASE64 must be set when Google Sheets is enabled}"
  set -- "$@" \
    --from-literal=GOOGLE_SHEETS_SPREADSHEET_ID="$GOOGLE_SHEETS_SPREADSHEET_ID" \
    --from-literal=GOOGLE_SERVICE_ACCOUNT_JSON_BASE64="$GOOGLE_SERVICE_ACCOUNT_JSON_BASE64"
fi

"$@" | kubectl apply -f -

echo "Applied Secret/ai-assistant-env in namespace $namespace"
