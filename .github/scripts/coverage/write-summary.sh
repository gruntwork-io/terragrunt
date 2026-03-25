#!/usr/bin/env bash

set -euo pipefail

REPORT="${1:-comparison-report.json}"
SUMMARY_FILE="${GITHUB_STEP_SUMMARY:-/dev/stdout}"

if [[ ! -f "$REPORT" ]]; then
	echo "No comparison report found, skipping summary."
	exit 0
fi

BASELINE=$(jq -r '.baseline' "$REPORT")
SIGNIFICANT=$(jq -r '.significant_change // true' "$REPORT")

# In significant-only mode, show a one-liner for insignificant changes
if [[ "${REPORT_MODE:-}" == "significant-only" && "$SIGNIFICANT" == "false" && "$BASELINE" != "true" ]]; then
	CURR=$(jq -r '.current_total' "$REPORT")
	DELTA=$(jq -r '.total_delta' "$REPORT")
	THRESHOLD=$(jq -r '.coverage_threshold // "n/a"' "$REPORT")
	echo "## :bar_chart: Coverage: ${CURR}% (delta: ${DELTA}%, below ${THRESHOLD}% threshold)" >>"$SUMMARY_FILE"
	exit 0
fi

if [[ "$BASELINE" == "true" ]]; then
	{
		echo "## Coverage Baseline Established"
		echo ""
		echo "Total: $(jq -r '.current_total' "$REPORT")%"
	} >>"$SUMMARY_FILE"
	exit 0
fi

CURR=$(jq -r '.current_total' "$REPORT")
PREV=$(jq -r '.previous_total' "$REPORT")
DELTA=$(jq -r '.total_delta' "$REPORT")

{
	echo "## :bar_chart: Coverage Diff"
	echo ""
	echo "| Metric | Value |"
	echo "|--------|-------|"
	echo "| Current | ${CURR}% |"
	echo "| Previous | ${PREV}% |"
	echo "| Delta | ${DELTA}% |"
	echo ""
} >>"$SUMMARY_FILE"

DROPS=$(jq -r '.top_drops[:5][] | "| \(.package) | \(.previous)% | \(.current)% | \(.delta)% |"' "$REPORT" 2>/dev/null || true)
if [[ -n "$DROPS" ]]; then
	{
		echo "### Top Drops"
		echo "| Package | Previous | Current | Delta |"
		echo "|---------|----------|---------|-------|"
		echo "$DROPS"
	} >>"$SUMMARY_FILE"
fi

GAINS=$(jq -r '.top_gains[:5][] | "| \(.package) | \(.previous)% | \(.current)% | +\(.delta)% |"' "$REPORT" 2>/dev/null || true)
if [[ -n "$GAINS" ]]; then
	{
		echo ""
		echo "### Top Gains"
		echo "| Package | Previous | Current | Delta |"
		echo "|---------|----------|---------|-------|"
		echo "$GAINS"
	} >>"$SUMMARY_FILE"
fi
