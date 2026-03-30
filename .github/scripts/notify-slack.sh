#!/usr/bin/env bash
# Common Slack notification script for CI comparison reports.
# Supports both coverage and duration report types via the "type" field in the JSON.
# Falls back to coverage format when "type" is missing (backwards compat).

set -euo pipefail

SLACK_WEBHOOK_URL="${SLACK_WEBHOOK_URL:-${COVERAGE_SLACK_WEBHOOK_URL:-}}"
SLACK_WEBHOOK_URL="${SLACK_WEBHOOK_URL:?Required: SLACK_WEBHOOK_URL or COVERAGE_SLACK_WEBHOOK_URL}"
REPORT="${1:-comparison-report.json}"
TAG_NAME="${TAG_NAME:-${BASE_REF:+${BASE_REF}...${HEAD_REF}}}"
TAG_NAME="${TAG_NAME:-adhoc}"
REPO="${REPO:-gruntwork-io/terragrunt}"
RUN_URL="${GITHUB_SERVER_URL:-https://github.com}/${REPO}/actions/runs/${GITHUB_RUN_ID:-0}"

if [[ ! -f "$REPORT" ]]; then
	echo "Error: report file '$REPORT' not found" >&2
	exit 1
fi

# Build payload entirely in jq to avoid shell interpolation of untrusted values.
# Detects report type from JSON "type" field; defaults to "coverage".
PAYLOAD=$(jq -n \
	--arg tag "$TAG_NAME" \
	--arg run_url "$RUN_URL" \
	--slurpfile report "$REPORT" '
	$report[0] as $r |
	($r.type // "coverage") as $type |

	if $type == "duration" then
		if $r.baseline then
			":stopwatch: *Duration Report: terragrunt \($tag)*\n\nBaseline established: \($r.current_total)s\n\n<\($run_url)|View workflow run>"
		else
			([$r.top_slowdowns[:5][] | "  \(.package): \(.previous)s -> \(.current)s (\(.delta)s, +\(.delta_pct)%)"] | join("\n")) as $slows |
			([$r.top_speedups[:3][] | "  \(.package): \(.previous)s -> \(.current)s (\(.delta)s, \(.delta_pct)%)"] | join("\n")) as $fasts |
			":stopwatch: *Duration Report: terragrunt \($tag)*\n\nTotal: \($r.current_total)s (was \($r.previous_total)s, delta: \($r.total_delta)s)" +
			(if ($slows | length) > 0 then "\n\nTop slowdowns:\n\($slows)" else "" end) +
			(if ($fasts | length) > 0 then "\n\nTop speedups:\n\($fasts)" else "" end) +
			"\n\n<\($run_url)|View workflow run>"
		end
	else
		# coverage (default)
		if $r.baseline then
			":bar_chart: *Coverage Report: terragrunt \($tag)*\n\nBaseline established: \($r.current_total)%\n\n<\($run_url)|View workflow run>"
		else
			([$r.top_drops[:5][] | "  \(.package): \(.previous)% -> \(.current)% (\(.delta)%)"] | join("\n")) as $drops |
			([$r.top_gains[:3][] | "  \(.package): \(.previous)% -> \(.current)% (+\(.delta)%)"] | join("\n")) as $gains |
			":bar_chart: *Coverage Report: terragrunt \($tag)*\n\nTotal: \($r.current_total)% (was \($r.previous_total)%, delta: \($r.total_delta)%)" +
			(if ($drops | length) > 0 then "\n\nTop drops:\n\($drops)" else "" end) +
			(if ($gains | length) > 0 then "\n\nTop gains:\n\($gains)" else "" end) +
			"\n\n<\($run_url)|View workflow run>"
		end
	end |
	{text: .}')

curl -sf -X POST \
	-H "Content-Type: application/json" \
	-d "$PAYLOAD" \
	"$SLACK_WEBHOOK_URL"

echo "Slack notification sent."
