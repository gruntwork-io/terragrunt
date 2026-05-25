#!/usr/bin/env bash

# Posts the combined weekly coverage + timing report to Slack via the configured
# webhook. Payload is built entirely in jq to avoid shell interpolation of
# untrusted report fields.

set -euo pipefail

COVERAGE_SLACK_WEBHOOK_URL="${COVERAGE_SLACK_WEBHOOK_URL:?Required environment variable COVERAGE_SLACK_WEBHOOK_URL}"
COV="${1:?Usage: notify-slack-weekly.sh <coverage-report.json> <timing-report.json>}"
TIM="${2:?Usage: notify-slack-weekly.sh <coverage-report.json> <timing-report.json>}"
TAG_NAME="${TAG_NAME:-weekly}"
REPO="${REPO:-gruntwork-io/terragrunt}"
RUN_URL="${GITHUB_SERVER_URL:-https://github.com}/${REPO}/actions/runs/${GITHUB_RUN_ID:-0}"

if [[ ! -f "$COV" ]]; then
	echo "Error: coverage report '$COV' not found" >&2
	exit 1
fi
if [[ ! -f "$TIM" ]]; then
	echo "Error: timing report '$TIM' not found" >&2
	exit 1
fi

PAYLOAD=$(jq -n \
	--arg tag "$TAG_NAME" \
	--arg run_url "$RUN_URL" \
	--slurpfile cov "$COV" \
	--slurpfile tim "$TIM" '
	($cov[0]) as $c |
	($tim[0]) as $t |

	# Coverage block
	(if $c.baseline then
		"Coverage baseline: \($c.current_total)%"
	else
		"Coverage: \($c.current_total)% (was \($c.previous_total)%, delta: \($c.total_delta)%)"
		+ (if (($c.top_drops // []) | length) > 0 then
			"\nTop drops:\n" + ([$c.top_drops[:5][] | "  \(.package): \(.previous)% -> \(.current)% (\(.delta)%)"] | join("\n"))
		   else "" end)
		+ (if (($c.top_gains // []) | length) > 0 then
			"\nTop gains:\n" + ([$c.top_gains[:5][] | "  \(.package): \(.previous)% -> \(.current)% (+\(.delta)%)"] | join("\n"))
		   else "" end)
	end) as $cov_block |

	# Timing block
	(if $t.baseline then
		"Runtime baseline: \($t.current_total_sec)s"
	else
		"Total runtime: \($t.current_total_sec)s (was \($t.previous_total_sec)s, delta: \($t.total_delta_sec)s)"
	end) as $rt_line |

	(if (($t.slow_packages // []) | length) > 0 then
		"Top slow packages:\n" + (
			[$t.slow_packages[] |
				. as $p |
				"  \($p.package): \($p.current_sec)s"
				+ (if $p.delta_sec != null then " (\($p.delta_sec)s)" else "" end)
				+ (if ($p.top_tests // []) | length > 0 then
					" | slowest: \($p.top_tests[0].name) (\($p.top_tests[0].current_sec)s)"
				   else "" end)
			] | join("\n"))
	else "" end) as $slow_pkgs |

	(if (($t.top_regressions // []) | length) > 0 then
		"Top runtime regressions:\n" + (
			[$t.top_regressions[] |
				"  \(.package): +\(.delta_sec)s (was \(.previous_sec)s)"
			] | join("\n"))
	else "" end) as $regressions |

	{
		text: (
			"*Weekly Coverage + Runtime: terragrunt \($tag)*\n\n"
			+ $cov_block
			+ "\n\n" + $rt_line
			+ (if $slow_pkgs != "" then "\n\n" + $slow_pkgs else "" end)
			+ (if $regressions != "" then "\n\n" + $regressions else "" end)
			+ "\n\n<\($run_url)|View workflow run>"
		)
	}')

curl -sf -X POST \
	-H "Content-Type: application/json" \
	-d "$PAYLOAD" \
	"$COVERAGE_SLACK_WEBHOOK_URL"

echo "Slack notification sent."
