#!/usr/bin/env bash
set -euo pipefail

date_value="${1:?Usage: restore.sh YYYY-MM-DD}"
[[ "$date_value" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || { echo "Date must be YYYY-MM-DD." >&2; exit 1; }
data_dir="${MYTODO_DATA_DIR:-/data}"
remote="${BACKUP_REMOTE:-r2:mytodo-backups}"
database="$data_dir/mytodo.db"
temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"' EXIT

command -v sqlite3 >/dev/null || { echo "sqlite3 is required." >&2; exit 1; }
command -v rclone >/dev/null || { echo "rclone is required." >&2; exit 1; }
[[ -f "$database" ]] || { echo "Database not found: $database" >&2; exit 1; }
printf 'Stop the mytodo server before restoring to prevent concurrent writes.\n' >&2

archive="$temporary/backup.db.gz"
incoming="$temporary/incoming.db"
previous="$temporary/previous.db"
rclone copyto "$remote/mytodo-$date_value.db.gz" "$archive"
gzip -dc "$archive" > "$incoming"

integrity="$(sqlite3 "$incoming" 'PRAGMA integrity_check;')"
[[ "$integrity" == "ok" ]] || { echo "Backup integrity check failed: $integrity" >&2; exit 1; }

row_counts() {
  local file="$1" table quoted
  while IFS= read -r table; do
    quoted="${table//\"/\"\"}"
    printf '%s=' "$table"
    sqlite3 "$file" "SELECT COUNT(*) FROM \"$quoted\";"
  done < <(sqlite3 -noheader "$file" "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name;")
}

before="$(row_counts "$database")"
expected="$(row_counts "$incoming")"
sqlite3 "$database" ".backup $previous"
if ! sqlite3 "$database" ".restore $incoming"; then
  sqlite3 "$database" ".restore $previous"
  echo "Restore failed; previous database was put back." >&2
  exit 1
fi
actual="$(row_counts "$database")"
if [[ "$actual" != "$expected" ]]; then
  sqlite3 "$database" ".restore $previous"
  echo "Post-restore row counts differ; previous database was put back." >&2
  exit 1
fi
printf 'Previous row counts:\n%s\nRestored row counts:\n%s\n' "$before" "$actual"