#!/usr/bin/env bash

set -euo pipefail

# Post the regressed and improved metrics of a benchmark report to Slack. Sends
# nothing when the report flags no metric.
#
# Usage: notify-slack.sh <report.json>
#
# Env:
#   COVERAGE_SLACK_WEBHOOK_URL   Webhook to post to, shared with the coverage reports.
#   REPO                         owner/name (default: gruntwork-io/terragrunt).
#   GITHUB_SERVER_URL, GITHUB_RUN_ID

readonly MAX_LINES_PER_VERDICT=10

function main {
	local -r webhook_url="${COVERAGE_SLACK_WEBHOOK_URL:?Required environment variable COVERAGE_SLACK_WEBHOOK_URL}"
	local -r report="${1:?Usage: notify-slack.sh <report.json>}"
	local -r repo="${REPO:-gruntwork-io/terragrunt}"
	local -r repo_url="${GITHUB_SERVER_URL:-https://github.com}/${repo}"
	local -r run_id="${GITHUB_RUN_ID:-0}"
	local -r max_lines="$MAX_LINES_PER_VERDICT"
	local flagged payload

	if [[ ! -f "$report" ]]; then
		echo "Error: report file '${report}' not found" >&2
		return 1
	fi

	flagged="$(jq -r '.regressions + .improvements' "$report")"

	if [[ "$flagged" == "0" ]]; then
		echo "Skipping Slack notification: no metric regressed or improved." >&2
		return 0
	fi

	# The payload is built in jq so that no value from the report reaches the shell.
	payload="$(
		jq -cn \
			--arg repo_url "$repo_url" \
			--arg run_id "$run_id" \
			--argjson max_lines "$max_lines" \
			--slurpfile report "$report" '
			$report[0] as $r |

			def sign: if . >= 0 then "+" else "" end;

			def lines($verdict):
				[
					$r.scenarios[] | .id as $id | .metrics[] | select(.verdict == $verdict) |
					{id: $id, name: "\(.source) \(.name)", delta: .deltaPct}
				] |
				sort_by(-(.delta | fabs)) |
				.[:$max_lines] |
				map("  `\(.id)` \(.name): \(.delta | sign)\(.delta * 10 | round / 10)%") |
				join("\n");

			lines("regression") as $regressions |
			lines("improvement") as $improvements |

			":stopwatch: *Benchmark (\($r.comparison)): terragrunt `\($r.baseSha[:12])`...`\($r.headSha[:12])`*\n\n" +
			"\($r.regressions) regressed and \($r.improvements) improved metrics" +
			(if ($regressions | length) > 0 then "\n\nRegressions:\n\($regressions)" else "" end) +
			(if ($improvements | length) > 0 then "\n\nImprovements:\n\($improvements)" else "" end) +
			"\n\n<\($repo_url)/compare/\($r.baseSha)...\($r.headSha)|View commits> | " +
			"<\($repo_url)/actions/runs/\($run_id)|View run and profiles>" |
			{text: .}'
	)"

	curl -sf -X POST \
		-H "Content-Type: application/json" \
		-d "$payload" \
		"$webhook_url"

	echo "Slack notification sent." >&2

	return 0
}

main "$@"
