#!/usr/bin/env bash

set -euo pipefail

SLACK_WEBHOOK_URL="${SLACK_WEBHOOK_URL:?Required environment variable SLACK_WEBHOOK_URL}"
REPORT="${1:-comparison-report.json}"
TAG_NAME="${TAG_NAME:-adhoc}"
REPO="${REPO:-gruntwork-io/terragrunt}"
RUN_URL="${GITHUB_SERVER_URL:-https://github.com}/${REPO}/actions/runs/${GITHUB_RUN_ID:-0}"

if [[ ! -f "$REPORT" ]]; then
	echo "Error: report file '$REPORT' not found"
	exit 1
fi

BASELINE=$(jq -r '.baseline' "$REPORT")
CURRENT=$(jq -r '.current_total' "$REPORT")

if [[ "$BASELINE" == "true" ]]; then
	TEXT=":bar_chart: *Coverage Report: terragrunt ${TAG_NAME}*\n\nBaseline established: ${CURRENT}%\n\n<${RUN_URL}|View workflow run>"
else
	PREVIOUS=$(jq -r '.previous_total' "$REPORT")
	DELTA=$(jq -r '.total_delta' "$REPORT")
	DROPS=$(jq -r '.top_drops[:5] | map("  \(.package): \(.previous)% → \(.current)% (\(.delta)%)") | join("\n")' "$REPORT")
	GAINS=$(jq -r '.top_gains[:3] | map("  \(.package): \(.previous)% → \(.current)% (+\(.delta)%)") | join("\n")' "$REPORT")

	TEXT=":bar_chart: *Coverage Report: terragrunt ${TAG_NAME}*\n\nTotal: ${CURRENT}% (was ${PREVIOUS}%, delta: ${DELTA}%)"
	if [[ -n "$DROPS" ]]; then
		TEXT="${TEXT}\n\nTop drops:\n${DROPS}"
	fi
	if [[ -n "$GAINS" ]]; then
		TEXT="${TEXT}\n\nTop gains:\n${GAINS}"
	fi
	TEXT="${TEXT}\n\n<${RUN_URL}|View workflow run>"
fi

PAYLOAD=$(jq -n --arg text "$TEXT" '{text: $text}')

curl -sf -X POST \
	-H "Content-Type: application/json" \
	-d "$PAYLOAD" \
	"$SLACK_WEBHOOK_URL"

echo "Slack notification sent."
