#!/usr/bin/env bash
set -euo pipefail

# Resolve the "current" and "previous" commits for the weekly coverage diff and
# write them (plus their dates) to $GITHUB_OUTPUT.
#
# Current is HEAD. Previous walks the trunk (origin/main) back WINDOW_DAYS, up to
# 2x that window, for the first commit that differs from HEAD. When none is found
# the report is current-only (same_commit=true).
#
# Env:
#   WINDOW_DAYS     Days back for the baseline (default: 7).
#   GITHUB_OUTPUT   Actions output file (defaults to stdout when run locally).

WINDOW_DAYS="${WINDOW_DAYS:-7}"
OUTPUT="${GITHUB_OUTPUT:-/dev/stdout}"

current_sha="$(git rev-parse HEAD)"

# Baseline walks origin/main. Fetch it in case this run was triggered from a
# branch where the remote-tracking ref is not present locally.
git fetch --no-tags --prune origin main || true
base_ref="origin/main"
git rev-parse --verify "$base_ref" >/dev/null 2>&1 || base_ref="HEAD"
echo "Baseline ref: ${base_ref}"

previous_sha=""
max_days=$((WINDOW_DAYS * 2))
for ((d = WINDOW_DAYS; d <= max_days; d++)); do
	candidate="$(git rev-list -1 --before="${d} days ago" "$base_ref" || true)"
	if [[ -n "$candidate" && "$candidate" != "$current_sha" ]]; then
		previous_sha="$candidate"
		echo "Selected previous SHA at ${d} days ago: ${previous_sha}"
		break
	fi
done

current_date="$(git show -s --format=%cs "$current_sha" 2>/dev/null || echo unknown)"
previous_date=""
same_commit=true
if [[ -n "$previous_sha" ]]; then
	previous_date="$(git show -s --format=%cs "$previous_sha" 2>/dev/null || echo unknown)"
	same_commit=false
else
	echo "No commit older than ${WINDOW_DAYS} days within ${max_days}-day walkback; reporting current only."
fi

{
	echo "current_sha=${current_sha}"
	echo "previous_sha=${previous_sha}"
	echo "current_date=${current_date}"
	echo "previous_date=${previous_date}"
	echo "same_commit=${same_commit}"
} >>"$OUTPUT"

echo "Current:  ${current_date} ${current_sha}"
echo "Previous: ${previous_date:-<none>} ${previous_sha:-<none>}"
