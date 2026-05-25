#!/usr/bin/env bash

# Renders the combined weekly coverage + timing report to $GITHUB_STEP_SUMMARY
# (or stdout when run outside Actions).

set -euo pipefail

COV="${1:?Usage: write-summary-weekly.sh <coverage-report.json> <timing-report.json>}"
TIM="${2:?Usage: write-summary-weekly.sh <coverage-report.json> <timing-report.json>}"
SUMMARY_FILE="${GITHUB_STEP_SUMMARY:-/dev/stdout}"

if [[ ! -f "$COV" ]]; then
	echo "No coverage comparison report at '$COV'; skipping summary." >&2
	exit 0
fi

write() { echo "$@" >>"$SUMMARY_FILE"; }

write "## Weekly Coverage + Timing Report"
write ""

# Coverage section -----------------------------------------------------------
COV_BASELINE=$(jq -r '.baseline' "$COV")
if [[ "$COV_BASELINE" == "true" ]]; then
	write "### Coverage"
	write ""
	write "Baseline established. Total: $(jq -r '.current_total' "$COV")%"
else
	write "### Coverage"
	write ""
	write "| Metric | Value |"
	write "|--------|-------|"
	write "| Current | $(jq -r '.current_total' "$COV")% |"
	write "| Previous | $(jq -r '.previous_total' "$COV")% |"
	write "| Delta | $(jq -r '.total_delta' "$COV")% |"
	write ""

	DROPS=$(jq -r '.top_drops[:5][] | "| \(.package) | \(.previous)% | \(.current)% | \(.delta)% |"' "$COV" 2>/dev/null || true)
	if [[ -n "$DROPS" ]]; then
		write "#### Top 5 coverage drops"
		write "| Package | Previous | Current | Delta |"
		write "|---------|----------|---------|-------|"
		write "$DROPS"
		write ""
	fi

	GAINS=$(jq -r '.top_gains[:5][] | "| \(.package) | \(.previous)% | \(.current)% | +\(.delta)% |"' "$COV" 2>/dev/null || true)
	if [[ -n "$GAINS" ]]; then
		write "#### Top 5 coverage gains"
		write "| Package | Previous | Current | Delta |"
		write "|---------|----------|---------|-------|"
		write "$GAINS"
		write ""
	fi
fi

# Timing section -------------------------------------------------------------
if [[ ! -f "$TIM" ]]; then
	write "### Test Runtime"
	write ""
	write "No timing report available."
	exit 0
fi

TIM_BASELINE=$(jq -r '.baseline' "$TIM")

write "### Test Runtime"
write ""
if [[ "$TIM_BASELINE" == "true" ]]; then
	printf "| Metric | Value |\n| --- | --- |\n| Current total | %ss |\n| Previous total | n/a (baseline) |\n\n" \
		"$(jq -r '.current_total_sec' "$TIM")" >>"$SUMMARY_FILE"
else
	{
		printf "| Metric | Value |\n| --- | --- |\n"
		printf "| Current total | %ss |\n" "$(jq -r '.current_total_sec' "$TIM")"
		printf "| Previous total | %ss |\n" "$(jq -r '.previous_total_sec' "$TIM")"
		printf "| Delta | %+ss |\n\n" "$(jq -r '.total_delta_sec' "$TIM")"
	} >>"$SUMMARY_FILE"
fi

write "#### Top 5 slowest packages"
write "| Package | Current | Previous | Delta |"
write "|---------|---------|----------|-------|"
jq -r '.slow_packages[] |
	"| \(.package) | \(.current_sec)s | \(if .previous_sec == null then "n/a" else "\(.previous_sec)s" end) | \(if .delta_sec == null then "n/a" else "\(.delta_sec)s" end) |"' "$TIM" >>"$SUMMARY_FILE"
write ""

write "#### Slowest tests per slow package"
SLOW_PKGS=$(jq -c '.slow_packages[]' "$TIM")
while IFS= read -r row; do
	[[ -z "$row" ]] && continue
	PKG=$(jq -r '.package' <<<"$row")
	write ""
	write "<details><summary><code>${PKG}</code></summary>"
	write ""
	write "| Test | Current | Previous | Delta |"
	write "|------|---------|----------|-------|"
	jq -r '.top_tests[] |
		"| `\(.name)` | \(.current_sec)s | \(if .previous_sec == null then "n/a" else "\(.previous_sec)s" end) | \(if .delta_sec == null then "n/a" else "\(.delta_sec)s" end) |"' <<<"$row" >>"$SUMMARY_FILE"
	write ""
	write "</details>"
done <<<"$SLOW_PKGS"

REGRESSIONS=$(jq -r '.top_regressions[] | "| \(.package) | \(.previous_sec)s | \(.current_sec)s | +\(.delta_sec)s |"' "$TIM" 2>/dev/null || true)
if [[ -n "$REGRESSIONS" ]]; then
	write ""
	write "#### Top 5 wall-time regressions (week over week)"
	write "| Package | Previous | Current | Delta |"
	write "|---------|----------|---------|-------|"
	write "$REGRESSIONS"
fi
