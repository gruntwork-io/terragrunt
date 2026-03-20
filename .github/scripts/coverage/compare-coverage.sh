#!/usr/bin/env bash

set -euo pipefail

CURRENT="${1:?Usage: compare-coverage.sh <current-summary.json> <previous-summary.json>}"
PREVIOUS="${2:?Usage: compare-coverage.sh <current-summary.json> <previous-summary.json>}"
OUTPUT="${3:-comparison-report.json}"

if [[ ! -f "$PREVIOUS" ]]; then
	echo "No previous coverage data found — establishing baseline."
	jq -n --argjson curr "$(cat "$CURRENT")" '{baseline: true, current_total: $curr.total_pct, previous_total: null, total_delta: null, top_drops: [], top_gains: []}' >"$OUTPUT"
	exit 0
fi

CURR_TOTAL=$(jq -r '.total_pct' "$CURRENT")
PREV_TOTAL=$(jq -r '.total_pct' "$PREVIOUS")
TOTAL_DELTA=$(echo "$CURR_TOTAL - $PREV_TOTAL" | bc -l)

# Compare per-package
PACKAGE_COMPARISON=$(jq -n \
	--argjson curr "$(jq '.packages' "$CURRENT")" \
	--argjson prev "$(jq '.packages' "$PREVIOUS")" \
	'[$prev | keys[], ($curr | keys[])] | unique | map(
		. as $pkg |
		{
			package: $pkg,
			current: ($curr[$pkg] // null),
			previous: ($prev[$pkg] // null),
			delta: (if ($curr[$pkg] != null) and ($prev[$pkg] != null)
				then ($curr[$pkg] - $prev[$pkg])
				else null end)
		}
	) | map(select(.delta != null))')

TOP_DROPS=$(echo "$PACKAGE_COMPARISON" | jq '[sort_by(.delta) | .[:10] | .[] | select(.delta < 0)]')
TOP_GAINS=$(echo "$PACKAGE_COMPARISON" | jq '[sort_by(-.delta) | .[:5] | .[] | select(.delta > 0)]')

jq -n \
	--argjson curr_total "$CURR_TOTAL" \
	--argjson prev_total "$PREV_TOTAL" \
	--argjson total_delta "$TOTAL_DELTA" \
	--argjson top_drops "$TOP_DROPS" \
	--argjson top_gains "$TOP_GAINS" \
	'{
		baseline: false,
		current_total: $curr_total,
		previous_total: $prev_total,
		total_delta: ($total_delta * 10 | round / 10),
		top_drops: $top_drops,
		top_gains: $top_gains
	}' >"$OUTPUT"

# Print summary table
echo "=== Coverage Comparison ==="
printf "Total: %.1f%% (was %.1f%%, delta: %+.1f%%)\n" "$CURR_TOTAL" "$PREV_TOTAL" "$TOTAL_DELTA"
echo ""

if [[ $(echo "$TOP_DROPS" | jq 'length') -gt 0 ]]; then
	echo "Top drops:"
	echo "$TOP_DROPS" | jq -r '.[] | "  \(.package): \(.previous)% → \(.current)% (\(.delta)%)"'
	echo ""
fi

if [[ $(echo "$TOP_GAINS" | jq 'length') -gt 0 ]]; then
	echo "Top gains:"
	echo "$TOP_GAINS" | jq -r '.[] | "  \(.package): \(.previous)% → \(.current)% (+\(.delta)%)"'
	echo ""
fi
