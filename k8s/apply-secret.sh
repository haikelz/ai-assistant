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

: "${SUMOPOD_API_KEY:?SUMOPOD_API_KEY must be set}"
: "${TELEGRAM_BOT_TOKEN:?TELEGRAM_BOT_TOKEN must be set}"
: "${TELEGRAM_USER_ID:?TELEGRAM_USER_ID must be set}"

kubectl create secret generic ai-assistant-env \
  --namespace "$namespace" \
  --dry-run=client \
  --output yaml \
  --from-literal=SUMOPOD_API_KEY="$SUMOPOD_API_KEY" \
  --from-literal=TELEGRAM_BOT_TOKEN="$TELEGRAM_BOT_TOKEN" \
  --from-literal=TELEGRAM_USER_ID="$TELEGRAM_USER_ID" \
  | kubectl apply -f -

echo "Applied Secret/ai-assistant-env in namespace $namespace"
