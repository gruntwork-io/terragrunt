#!/usr/bin/env bash

set -euo pipefail

CURRENT="${1:?Usage: compare-durations.sh <current-summary.json> <previous-summary.json>}"
PREVIOUS="${2:?Usage: compare-durations.sh <current-summary.json> <previous-summary.json>}"
OUTPUT="${3:-duration-comparison-report.json}"
HTML_OUTPUT="${OUTPUT%.json}.html"

if [[ ! -f "$CURRENT" ]]; then
	echo "Error: current duration file '$CURRENT' not found" >&2
	exit 1
fi

if [[ ! -f "$PREVIOUS" ]]; then
	echo "No previous duration data found — establishing baseline."
	jq -n --argjson curr "$(cat "$CURRENT")" \
		'{type: "duration", baseline: true, current_total: $curr.total_time, previous_total: null, total_delta: null, top_slowdowns: [], top_speedups: []}' >"$OUTPUT"
	exit 0
fi

CURR_TOTAL=$(jq -r '.total_time' "$CURRENT")
PREV_TOTAL=$(jq -r '.total_time' "$PREVIOUS")
TOTAL_DELTA=$(echo "$CURR_TOTAL - $PREV_TOTAL" | bc -l)

CURR_REF=$(jq -r '.ref // "head"' "$CURRENT")
PREV_REF=$(jq -r '.ref // "base"' "$PREVIOUS")
CURR_COMMIT=$(jq -r '.commit // "unknown"' "$CURRENT")
PREV_COMMIT=$(jq -r '.commit // "unknown"' "$PREVIOUS")
CURR_TS=$(jq -r '.timestamp // ""' "$CURRENT")
PREV_TS=$(jq -r '.timestamp // ""' "$PREVIOUS")

# Compare all packages — same jq pattern as compare-coverage.sh
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
				else null end),
			delta_pct: (if ($curr[$pkg] != null) and ($prev[$pkg] != null) and ($prev[$pkg] > 0)
				then ((($curr[$pkg] - $prev[$pkg]) / $prev[$pkg] * 100) * 10 | round / 10)
				else null end)
		}
	)')

# Dual threshold: >20% slower AND >5s absolute increase
TOP_SLOWDOWNS=$(echo "$PACKAGE_COMPARISON" | jq '[map(select(.delta != null and .delta > 5 and .delta_pct != null and .delta_pct > 20)) | sort_by(-.delta) | .[:10] | .[]]')
TOP_SPEEDUPS=$(echo "$PACKAGE_COMPARISON" | jq '[map(select(.delta != null and .delta < -5 and .delta_pct != null and .delta_pct < -20)) | sort_by(.delta) | .[:5] | .[]]')

jq -n \
	--argjson curr_total "$CURR_TOTAL" \
	--argjson prev_total "$PREV_TOTAL" \
	--argjson total_delta "$TOTAL_DELTA" \
	--argjson top_slowdowns "$TOP_SLOWDOWNS" \
	--argjson top_speedups "$TOP_SPEEDUPS" \
	'{
		type: "duration",
		baseline: false,
		current_total: $curr_total,
		previous_total: $prev_total,
		total_delta: ($total_delta * 10 | round / 10),
		top_slowdowns: $top_slowdowns,
		top_speedups: $top_speedups
	}' >"$OUTPUT"

# Generate HTML report — same structure as coverage comparison
{
	cat <<-HEADER
	<!DOCTYPE html>
	<html><head>
	<meta charset="utf-8">
	<title>Duration Comparison</title>
	<style>
	body { font-family: monospace; margin: 2em; }
	table { border-collapse: collapse; width: 100%; }
	th, td { border: 1px solid #ccc; padding: 6px 12px; text-align: right; }
	th { background: #f5f5f5; }
	td:first-child, th:first-child { text-align: left; }
	tr.total { font-weight: bold; border-top: 2px solid #333; }
	tr.new td, tr.removed td { font-style: italic; }
	.slow { color: #c00; }
	.fast { color: #080; }
	</style>
	</head><body>
	<h2>Duration Comparison</h2>
	<table>
	<tr><td>Base</td><td>${PREV_REF} (${PREV_COMMIT:0:12})</td><td>${PREV_TS}</td></tr>
	<tr><td>Head</td><td>${CURR_REF} (${CURR_COMMIT:0:12})</td><td>${CURR_TS}</td></tr>
	</table>
	<br>
	<table>
	<tr><th>Package</th><th>Base (s)</th><th>Head (s)</th><th>Delta (s)</th><th>Delta %</th></tr>
	HEADER

	printf '<tr class="total"><td>TOTAL</td><td>%.1f</td><td>%.1f</td><td>%+.1f</td><td>—</td></tr>\n' "$PREV_TOTAL" "$CURR_TOTAL" "$TOTAL_DELTA"

	echo "$PACKAGE_COMPARISON" | jq -r 'sort_by(.package) | .[] |
		if .previous == null then
			"<tr class=\"new\"><td>\(.package)</td><td>—</td><td>\(.current)s</td><td>new</td><td>—</td></tr>"
		elif .current == null then
			"<tr class=\"removed\"><td>\(.package)</td><td>\(.previous)s</td><td>—</td><td>removed</td><td>—</td></tr>"
		else
			"<tr><td>\(.package)</td><td>\(.previous)</td><td>\(.current)</td><td>\(.delta)</td><td>\(.delta_pct)%</td></tr>"
		end'

	cat <<-FOOTER
	</table>
	</body></html>
	FOOTER
} >"$HTML_OUTPUT"

# Print summary
echo "=== Duration Comparison ==="
printf "Total: %.1fs (was %.1fs, delta: %+.1fs)\n" "$CURR_TOTAL" "$PREV_TOTAL" "$TOTAL_DELTA"
echo ""

if [[ $(echo "$TOP_SLOWDOWNS" | jq 'length') -gt 0 ]]; then
	echo "Top slowdowns (>20% and >5s):"
	echo "$TOP_SLOWDOWNS" | jq -r '.[] | "  \(.package): \(.previous)s -> \(.current)s (\(.delta)s, +\(.delta_pct)%)"'
	echo ""
fi

if [[ $(echo "$TOP_SPEEDUPS" | jq 'length') -gt 0 ]]; then
	echo "Top speedups:"
	echo "$TOP_SPEEDUPS" | jq -r '.[] | "  \(.package): \(.previous)s -> \(.current)s (\(.delta)s, \(.delta_pct)%)"'
	echo ""
fi

echo "HTML report: $HTML_OUTPUT"
