#!/usr/bin/env bash

set -euo pipefail

REPORT="${1:-comparison-report.json}"
BASE_REF="${BASE_REF:-base}"
HEAD_REF="${HEAD_REF:-head}"
SUMMARY_FILE="${GITHUB_STEP_SUMMARY:-/dev/stdout}"

if [[ ! -f "$REPORT" ]]; then
	echo "No comparison report found, skipping summary."
	exit 0
fi

# Sanitize user inputs for markdown injection
BASE_REF=$(tr -d '<>|`\n\r' <<<"$BASE_REF")
HEAD_REF=$(tr -d '<>|`\n\r' <<<"$HEAD_REF")

CURR=$(jq -r '.current_total' "$REPORT")
PREV=$(jq -r '.previous_total' "$REPORT")
DELTA=$(jq -r '.total_delta' "$REPORT")

{
	echo "## Coverage Comparison: ${BASE_REF} vs ${HEAD_REF}"
	echo ""
	echo "| Ref | Coverage |"
	echo "|-----|----------|"
	echo "| ${BASE_REF} (base) | ${PREV}% |"
	echo "| ${HEAD_REF} (head) | ${CURR}% |"
	echo "| Delta | ${DELTA}% |"
	echo ""
} >>"$SUMMARY_FILE"

DROPS=$(jq -r '.top_drops[:10][] | "| \(.package) | \(.previous)% | \(.current)% | \(.delta)% |"' "$REPORT" 2>/dev/null || true)
if [[ -n "$DROPS" ]]; then
	{
		echo "### Top Drops"
		echo "| Package | Base | Head | Delta |"
		echo "|---------|------|------|-------|"
		echo "$DROPS"
	} >>"$SUMMARY_FILE"
fi

GAINS=$(jq -r '.top_gains[:5][] | "| \(.package) | \(.previous)% | \(.current)% | +\(.delta)% |"' "$REPORT" 2>/dev/null || true)
if [[ -n "$GAINS" ]]; then
	{
		echo ""
		echo "### Top Gains"
		echo "| Package | Base | Head | Delta |"
		echo "|---------|------|------|-------|"
		echo "$GAINS"
	} >>"$SUMMARY_FILE"
fi
