#!/usr/bin/env bash
set -euo pipefail

app="${FLY_APP_NAME:-kid-life-archive}"
region="${FLY_REGION:-sea}"

command -v fly >/dev/null || { echo "Install flyctl first." >&2; exit 1; }
command -v openssl >/dev/null || { echo "Install openssl first." >&2; exit 1; }
: "${STRIPE_PAYMENT_LINK:?Set STRIPE_PAYMENT_LINK to a Stripe customer-chosen-amount Payment Link.}"
: "${SMTP_HOST:?Set SMTP_HOST for passwordless signup email delivery.}"
: "${SMTP_FROM:?Set SMTP_FROM for passwordless signup email delivery.}"

if ! fly apps list | awk '{print $1}' | grep -Fxq "$app"; then
  fly apps create "$app"
fi

if ! fly volumes list -a "$app" | grep -Fq mytodo_data; then
  fly volumes create mytodo_data --region "$region" --size 1 -a "$app"
fi

if [[ ! -f fly.toml ]]; then
  sed "s/__FLY_APP_NAME__/$app/g; s/primary_region = \"sea\"/primary_region = \"$region\"/" fly.toml.example > fly.toml
fi

secret="$(fly secrets list -a "$app" | awk 'NR > 1 {print $1}' | grep -Fx SESSION_SECRET || true)"
if [[ -z "$secret" ]]; then
  fly secrets set "SESSION_SECRET=$(openssl rand -hex 32)" -a "$app"
fi
fly secrets set "STRIPE_PAYMENT_LINK=$STRIPE_PAYMENT_LINK" "SMTP_HOST=$SMTP_HOST" "SMTP_PORT=${SMTP_PORT:-587}" "SMTP_FROM=$SMTP_FROM" -a "$app"
  fly secrets set "MYTODO_PUBLIC_URL=${MYTODO_PUBLIC_URL:-https://${app}.fly.dev}" -a "$app"
if [[ -n "${SMTP_USER:-}" ]]; then
  : "${SMTP_PASSWORD:?Set SMTP_PASSWORD when SMTP_USER is configured.}"
  fly secrets set "SMTP_USER=$SMTP_USER" "SMTP_PASSWORD=$SMTP_PASSWORD" -a "$app"
fi

fly deploy -a "$app"
printf 'Deployed https://%s.fly.dev\n' "$app"
printf 'The attached Fly volume is persistent and billed; Fly does not provide a free persistent-volume tier.\n'