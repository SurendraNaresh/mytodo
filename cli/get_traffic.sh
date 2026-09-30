#!/usr/bin/env bash
# get_repotraffic.sh <owner>
# Requires: curl, jq
# Usage: ./get_repotraffic.sh <owner>

set -euo pipefail

OWNER="${1:-}"
TOKEN="${GITHUB_TOKEN:-}"

if [[ -z "$OWNER" ]]; then
  echo "Usage: $0 <owner>"
  exit 1
fi

if [[ -z "$TOKEN" ]]; then
  echo "Error: GITHUB_TOKEN environment variable not set."
  exit 1
fi

# Fetch all repos for the owner
repos=$(curl -s -H "Authorization: Bearer $TOKEN" \
              -H "Accept: application/vnd.github+json" \
              "https://api.github.com/users/$OWNER/repos?per_page=100" | jq -r '.[].name')

if [[ -z "$repos" ]]; then
  echo "No repositories found for owner: $OWNER"
  exit 0
fi

results=$(mktemp)

for repo in $repos; do
  clones=$(curl -s -H "Authorization: Bearer $TOKEN" \
                 -H "Accept: application/vnd.github+json" \
                 "https://api.github.com/repos/$OWNER/$repo/traffic/clones" \
           | jq -r '.count // 0')
  echo -e "$clones\t$repo" >> "$results"
done

# Sort by clone count descending
echo "Repository traffic (clones):"
sort -nr "$results" | awk -F'\t' '{printf "%-30s %d\n", $2, $1}'

rm -f "$results"
