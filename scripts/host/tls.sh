#!/usr/bin/env bash
set -euo pipefail

domain="${1:?Usage: tls.sh your-domain.example}"
upstream="${MYTODO_UPSTREAM:-127.0.0.1:8080}"
command -v caddy >/dev/null || { echo "Install Caddy first." >&2; exit 1; }

config="$(mktemp)"
trap 'rm -f "$config"' EXIT
cat > "$config" <<EOF
$domain {
  encode zstd gzip
  reverse_proxy $upstream
}
EOF
sudo install -m 0644 "$config" /etc/caddy/Caddyfile
sudo systemctl reload caddy
printf 'Caddy is configured for https://%s; confirm DNS points to this VM.\n' "$domain"