#!/usr/bin/env bash

set -euo pipefail

COVERAGE_CHANGE_THRESHOLD="${COVERAGE_CHANGE_THRESHOLD:-3}"

CURRENT="${1:?Usage: compare-coverage.sh <current-summary.json> <previous-summary.json>}"
PREVIOUS="${2:?Usage: compare-coverage.sh <current-summary.json> <previous-summary.json>}"
OUTPUT="${3:-comparison-report.json}"
HTML_OUTPUT="${OUTPUT%.json}.html"

if [[ ! -f "$CURRENT" ]]; then
	echo "Error: current coverage file '$CURRENT' not found" >&2
	exit 1
fi

if [[ ! -f "$PREVIOUS" ]]; then
	echo "No previous coverage data found — establishing baseline."
	jq -n --argjson curr "$(cat "$CURRENT")" '{baseline: true, current_total: $curr.total_pct, previous_total: null, total_delta: null, top_drops: [], top_gains: []}' >"$OUTPUT"
	exit 0
fi

CURR_TOTAL=$(jq -r '.total_pct' "$CURRENT")
PREV_TOTAL=$(jq -r '.total_pct' "$PREVIOUS")
TOTAL_DELTA=$(bc -l <<<"$CURR_TOTAL - $PREV_TOTAL")

CURR_REF=$(jq -r '.ref // "head"' "$CURRENT")
PREV_REF=$(jq -r '.ref // "base"' "$PREVIOUS")
CURR_COMMIT=$(jq -r '.commit // "unknown"' "$CURRENT")
PREV_COMMIT=$(jq -r '.commit // "unknown"' "$PREVIOUS")
CURR_TS=$(jq -r '.timestamp // ""' "$CURRENT")
PREV_TS=$(jq -r '.timestamp // ""' "$PREVIOUS")

# Compare all packages
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
				then (($curr[$pkg] - $prev[$pkg]) * 10 | round / 10)
				else null end)
		}
	)')

TOP_DROPS=$(jq '[map(select(.delta != null and .delta < 0)) | sort_by(.delta) | .[:10] | .[]]' <<<"$PACKAGE_COMPARISON")
TOP_GAINS=$(jq '[map(select(.delta != null and .delta > 0)) | sort_by(-.delta) | .[:5] | .[]]' <<<"$PACKAGE_COMPARISON")

jq -n \
	--argjson curr_total "$CURR_TOTAL" \
	--argjson prev_total "$PREV_TOTAL" \
	--argjson total_delta "$TOTAL_DELTA" \
	--argjson threshold "${COVERAGE_CHANGE_THRESHOLD}" \
	--argjson top_drops "$TOP_DROPS" \
	--argjson top_gains "$TOP_GAINS" \
	'{
		baseline: false,
		current_total: $curr_total,
		previous_total: $prev_total,
		total_delta: ($total_delta * 10 | round / 10),
		coverage_threshold: $threshold,
		significant_change: ((($total_delta * 10 | round / 10) | fabs) >= $threshold),
		top_drops: $top_drops,
		top_gains: $top_gains
	}' >"$OUTPUT"

# Generate HTML report
{
	cat <<-HEADER
		<!DOCTYPE html>
		<html><head>
		<meta charset="utf-8">
		<title>Coverage Comparison</title>
		<style>
		body { font-family: monospace; margin: 2em; }
		table { border-collapse: collapse; width: 100%; }
		th, td { border: 1px solid #ccc; padding: 6px 12px; text-align: right; }
		th { background: #f5f5f5; }
		td:first-child, th:first-child { text-align: left; }
		tr.total { font-weight: bold; border-top: 2px solid #333; }
		tr.new td, tr.removed td { font-style: italic; }
		</style>
		</head><body>
		<h2>Coverage Comparison</h2>
		<table>
		<tr><td>Base</td><td>${PREV_REF} (${PREV_COMMIT:0:12})</td><td>${PREV_TS}</td></tr>
		<tr><td>Head</td><td>${CURR_REF} (${CURR_COMMIT:0:12})</td><td>${CURR_TS}</td></tr>
		</table>
		<br>
		<table>
		<tr><th>Package</th><th>Base %</th><th>Head %</th><th>Delta</th></tr>
	HEADER

	# Total row
	printf '<tr class="total"><td>TOTAL</td><td>%.1f%%</td><td>%.1f%%</td><td>%+.1f%%</td></tr>\n' "$PREV_TOTAL" "$CURR_TOTAL" "$TOTAL_DELTA"

	# All packages sorted by name
	jq -r 'sort_by(.package) | .[] |
		if .previous == null then
			"<tr class=\"new\"><td>\(.package)</td><td>—</td><td>\(.current)%</td><td>new</td></tr>"
		elif .current == null then
			"<tr class=\"removed\"><td>\(.package)</td><td>\(.previous)%</td><td>—</td><td>removed</td></tr>"
		else
			"<tr><td>\(.package)</td><td>\(.previous)%</td><td>\(.current)%</td><td>\(.delta)%</td></tr>"
		end' <<<"$PACKAGE_COMPARISON"

	cat <<-FOOTER
		</table>
		</body></html>
	FOOTER
} >"$HTML_OUTPUT"

# Print summary table
echo "=== Coverage Comparison ==="
printf "Total: %.1f%% (was %.1f%%, delta: %+.1f%%)\n" "$CURR_TOTAL" "$PREV_TOTAL" "$TOTAL_DELTA"
echo ""

if [[ $(jq 'length' <<<"$TOP_DROPS") -gt 0 ]]; then
	echo "Top drops:"
	jq -r '.[] | "  \(.package): \(.previous)% → \(.current)% (\(.delta)%)"' <<<"$TOP_DROPS"
	echo ""
fi

if [[ $(jq 'length' <<<"$TOP_GAINS") -gt 0 ]]; then
	echo "Top gains:"
	jq -r '.[] | "  \(.package): \(.previous)% → \(.current)% (+\(.delta)%)"' <<<"$TOP_GAINS"
	echo ""
fi

echo "HTML report: $HTML_OUTPUT"
