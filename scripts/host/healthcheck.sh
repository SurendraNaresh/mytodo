#!/usr/bin/env bash
set -euo pipefail

url="${MYTODO_HEALTHCHECK_URL:-https://kid-life-archive.fly.dev/healthz}"
message="mytodo health check failed: $url"

if curl --fail --silent --show-error --max-time 10 "$url" >/dev/null; then
  exit 0
fi

if [[ -n "${TELEGRAM_BOT_TOKEN:-}" && -n "${TELEGRAM_CHAT_ID:-}" ]]; then
  curl --fail --silent --show-error --max-time 10 \
    "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
    --data-urlencode "chat_id=$TELEGRAM_CHAT_ID" --data-urlencode "text=$message" >/dev/null || true
elif [[ -n "${ALERT_WEBHOOK_URL:-}" ]]; then
  payload="$(printf '%s' "$message" | jq -Rs '{content: .}')"
  curl --fail --silent --show-error --max-time 10 -H 'Content-Type: application/json' \
    -d "$payload" "$ALERT_WEBHOOK_URL" >/dev/null || true
fi
exit 1