#!/usr/bin/env bash
set -euo pipefail

data_dir="${MYTODO_DATA_DIR:-/data}"
remote="${BACKUP_REMOTE:-r2:mytodo-backups}"
database="$data_dir/mytodo.db"
stamp="$(date -u +%F)"
temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"' EXIT

command -v sqlite3 >/dev/null || { echo "sqlite3 is required." >&2; exit 1; }
command -v rclone >/dev/null || { echo "rclone is required." >&2; exit 1; }
[[ -f "$database" ]] || { echo "Database not found: $database" >&2; exit 1; }

snapshot="$temporary/mytodo-$stamp.db"
sqlite3 "$database" ".backup $snapshot"
gzip -n "$snapshot"
rclone copyto "$snapshot.gz" "$remote/mytodo-$stamp.db.gz"

mapfile -t backups < <(rclone lsf --files-only "$remote" | grep -E '^mytodo-[0-9]{4}-[0-9]{2}-[0-9]{2}\.db\.gz$' | sort || true)
excess=$((${#backups[@]} - 30))
if (( excess > 0 )); then
  for ((index = 0; index < excess; index++)); do
    rclone deletefile "$remote/${backups[$index]}"
  done
fi
printf 'Uploaded and retained backup mytodo-%s.db.gz\n' "$stamp"