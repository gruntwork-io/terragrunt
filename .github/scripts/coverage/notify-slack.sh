#!/usr/bin/env bash

set -euo pipefail

COVERAGE_SLACK_WEBHOOK_URL="${COVERAGE_SLACK_WEBHOOK_URL:?Required environment variable COVERAGE_SLACK_WEBHOOK_URL}"
REPORT="${1:-comparison-report.json}"
TAG_NAME="${TAG_NAME:-adhoc}"
REPO="${REPO:-gruntwork-io/terragrunt}"
RUN_URL="${GITHUB_SERVER_URL:-https://github.com}/${REPO}/actions/runs/${GITHUB_RUN_ID:-0}"

if [[ ! -f "$REPORT" ]]; then
	echo "Error: report file '$REPORT' not found"
	exit 1
fi

# Build payload entirely in jq to avoid shell interpolation of untrusted values
PAYLOAD=$(jq -n \
	--arg tag "$TAG_NAME" \
	--arg run_url "$RUN_URL" \
	--slurpfile report "$REPORT" '
	$report[0] as $r |
	if $r.baseline then
		":bar_chart: *Coverage Report: terragrunt \($tag)*\n\nBaseline established: \($r.current_total)%\n\n<\($run_url)|View workflow run>"
	else
		([$r.top_drops[:5][] | "  \(.package): \(.previous)% → \(.current)% (\(.delta)%)"] | join("\n")) as $drops |
		([$r.top_gains[:3][] | "  \(.package): \(.previous)% → \(.current)% (+\(.delta)%)"] | join("\n")) as $gains |
		":bar_chart: *Coverage Report: terragrunt \($tag)*\n\nTotal: \($r.current_total)% (was \($r.previous_total)%, delta: \($r.total_delta)%)" +
		(if ($drops | length) > 0 then "\n\nTop drops:\n\($drops)" else "" end) +
		(if ($gains | length) > 0 then "\n\nTop gains:\n\($gains)" else "" end) +
		"\n\n<\($run_url)|View workflow run>"
	end |
	{text: .}')

curl -sf -X POST \
	-H "Content-Type: application/json" \
	-d "$PAYLOAD" \
	"$COVERAGE_SLACK_WEBHOOK_URL"

echo "Slack notification sent."
