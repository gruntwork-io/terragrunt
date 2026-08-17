#!/usr/bin/env bash
set -euo pipefail

# Single entrypoint for the weekly security scan report. Only the `scan` and
# `summary` subcommands know about the scanner (currently Trivy); the summary,
# report, and notification formats are scanner-agnostic so the implementation
# can be swapped without changing the workflow contract.
#
# Subcommands:
#   scan <src-dir> <out.json> [trivy args...]  Run `trivy fs` over <src-dir> and write the
#                                    raw JSON report to <out.json>. Extra args are passed
#                                    through to trivy (e.g. --skip-db-update).
#   summary <scan.json> <out.json>   Normalize a raw scan report into a flat findings
#                                    summary keyed for diffing, with per-severity counts.
#   compare <cur> <prev> [out]       Diff two summaries -> report JSON with new and fixed
#                                    findings. A missing/empty previous summary yields a
#                                    baseline (current-only) report.
#   render <report.json>             Render the report as Markdown to $GITHUB_STEP_SUMMARY
#                                    (or stdout).
#   payload <report.json>            Print the Slack message payload JSON to stdout.
#   notify <report.json>             Post the report to Slack ($SECURITY_SCAN_SLACK_WEBHOOK_URL).
#   notify-failure                   Post a run-failure notice to Slack so a broken scan
#                                    is not a silent missed week.
#
# Env knobs — SECURITY_SCAN_* configure the scanner-agnostic reporting;
# TRIVY_* configure the scanner itself (intentionally shadowing trivy's native
# env config; the explicit flags passed by `scan` always win):
#   SECURITY_SCAN_RENDER_LIMIT  Max current findings rendered in the step summary (default: 50).
#   SECURITY_SCAN_NOTIFY_LIMIT  Max new/fixed findings listed in the Slack message (default: 10).
#   TRIVY_SCANNERS      Scanners for `scan` (default: vuln,misconfig,secret).
#   TRIVY_SKIP_DIRS     Comma-separated dirs `scan` skips (default: test and
#                       docs/src/fixtures, whose intentionally-simple fixtures
#                       would drown the report).
#   TRIVY_TIMEOUT       Trivy scan timeout (default: 15m).
#
# Pass a missing-previous path as /nonexistent to `compare` to get a baseline report.

SELF="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"

usage() {
	sed -n '/^# Single entrypoint/,/to get a baseline report\.$/p' "$SELF"
}

now_utc() { date -u +%Y-%m-%dT%H:%M:%SZ; }
this_commit() { echo "${GITHUB_SHA:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}"; }

# Shared jq defs: severity normalization/ranking and the one-line finding format.
jq_lib() {
	cat <<-'JQ'
		def sev: if . == null then "UNKNOWN"
			elif (["CRITICAL","HIGH","MEDIUM","LOW","UNKNOWN"] | index(.)) then .
			else "UNKNOWN" end;
		def sevrank: {"CRITICAL": 0, "HIGH": 1, "MEDIUM": 2, "LOW": 3, "UNKNOWN": 4}[.] // 9;
		def counts_line: . as $c |
			(["CRITICAL","HIGH","MEDIUM","LOW","UNKNOWN"]
				| map(. as $s | select(($c[$s] // 0) > 0) | "\($c[$s]) \($s)")
				| join(", ")) as $by_sev |
			"\($c.total // 0) total" + (if $by_sev != "" then " (\($by_sev))" else "" end);
		def finding_line:
			"[\(.severity)] \(.class) \(.id)"
			+ (if .pkg != "" then " \(.pkg)" else "" end)
			+ (if .installed != "" then " \(.installed)" else "" end)
			+ (if .fixed != "" then " -> \(.fixed)" else "" end)
			+ " (\(.target))";
	JQ
}

# Run `trivy fs` over a source tree and write the raw JSON report
cmd_scan() {
	local src="${1:?Usage: security-scan-report.sh scan <src-dir> <out.json> [trivy args...]}"
	local out="${2:?Usage: security-scan-report.sh scan <src-dir> <out.json> [trivy args...]}"
	shift 2

	if ! command -v trivy >/dev/null 2>&1; then
		echo "Error: trivy not found on PATH; install it (e.g. aquasecurity/setup-trivy) first." >&2
		exit 1
	fi

	mkdir -p "$(dirname "$out")"

	# Findings never fail the scan (--exit-code 0): the diff report is the deliverable.
	trivy fs \
		--scanners "${TRIVY_SCANNERS:-vuln,misconfig,secret}" \
		--skip-dirs "${TRIVY_SKIP_DIRS:-test,docs/src/fixtures}" \
		--timeout "${TRIVY_TIMEOUT:-15m}" \
		--format json \
		--output "$out" \
		--exit-code 0 \
		--no-progress \
		"$@" \
		"$src"

	echo "Scan:  $src"
	echo "Report: $out ($(jq '[.Results[]? | ((.Vulnerabilities // []) + (.Misconfigurations // []) + (.Secrets // []))] | flatten | length' "$out") raw findings)"
}

# Normalize a raw Trivy report into a flat, diffable findings summary
cmd_summary() {
	local input="${1:?Usage: security-scan-report.sh summary <trivy.json> <out.json>}"
	local output="${2:?Usage: security-scan-report.sh summary <trivy.json> <out.json>}"

	if [[ ! -f "$input" ]]; then
		echo "Error: trivy report '$input' not found" >&2
		exit 1
	fi

	if ! jq empty "$input" 2>/dev/null; then
		echo "Error: trivy report '$input' is not valid JSON" >&2
		exit 1
	fi

	mkdir -p "$(dirname "$output")"

	# One record per unique (class, target, pkg, id); passing misconfigurations are dropped.
	jq \
		--arg commit "$(this_commit)" \
		--arg timestamp "$(now_utc)" \
		"$(jq_lib)"'
		[.Results[]? as $r |
			(($r.Vulnerabilities // [])[] | {
				class: "vuln",
				id: (.VulnerabilityID // ""),
				severity: (.Severity | sev),
				target: ($r.Target // ""),
				pkg: (.PkgName // ""),
				installed: (.InstalledVersion // ""),
				fixed: (.FixedVersion // ""),
				title: (.Title // "")
			}),
			(($r.Misconfigurations // [])[] | select((.Status // "FAIL") == "FAIL") | {
				class: "misconfig",
				id: (.AVDID // .ID // ""),
				severity: (.Severity | sev),
				target: ($r.Target // ""),
				pkg: "",
				installed: "",
				fixed: "",
				title: (.Title // "")
			}),
			(($r.Secrets // [])[] | {
				class: "secret",
				id: (.RuleID // ""),
				severity: (.Severity | sev),
				target: ($r.Target // ""),
				pkg: "",
				installed: "",
				fixed: "",
				title: (.Title // "")
			})
		]
		| map(. + {key: "\(.class)|\(.target)|\(.pkg)|\(.id)"})
		| unique_by(.key)
		| sort_by([(.severity | sevrank), .key])
		| {
			commit: $commit,
			timestamp: $timestamp,
			counts: (reduce .[] as $f (
				{CRITICAL: 0, HIGH: 0, MEDIUM: 0, LOW: 0, UNKNOWN: 0, total: 0};
				.[$f.severity] += 1 | .total += 1)),
			findings: .
		}' "$input" >"$output"

	echo "Summary: $output ($(jq -r '.counts.total' "$output") findings)"
}

# Diff two findings summaries into a new/fixed report
cmd_compare() {
	local current="${1:?Usage: security-scan-report.sh compare <current.json> <previous.json> [output.json]}"
	local previous="${2:?Usage: security-scan-report.sh compare <current.json> <previous.json> [output.json]}"
	local output="${3:-security-scan-report.json}"

	if [[ ! -f "$current" ]]; then
		echo "Error: current summary '$current' not found" >&2
		exit 1
	fi

	if [[ ! -s "$previous" ]]; then
		echo "No previous scan data found: establishing baseline."
		jq '{
			baseline: true,
			current_counts: .counts,
			previous_counts: null,
			new: [],
			fixed: [],
			unchanged: null,
			current_findings: .findings
		}' "$current" >"$output"
		return 0
	fi

	jq -n \
		--slurpfile cur "$current" \
		--slurpfile prev "$previous" '
		($cur[0]) as $c |
		($prev[0]) as $p |
		($c.findings | map({(.key): true}) | add // {}) as $ckeys |
		($p.findings | map({(.key): true}) | add // {}) as $pkeys |
		{
			baseline: false,
			current_counts: $c.counts,
			previous_counts: $p.counts,
			new: [$c.findings[] | select($pkeys[.key] | not)],
			fixed: [$p.findings[] | select($ckeys[.key] | not)],
			unchanged: ([$c.findings[] | select($pkeys[.key])] | length),
			current_findings: $c.findings
		}' >"$output"

	echo "Report: $output ($(jq -r '.new | length' "$output") new, $(jq -r '.fixed | length' "$output") fixed)"
}

# Render the report as Markdown to $GITHUB_STEP_SUMMARY (or stdout)
cmd_render() {
	local report="${1:?Usage: security-scan-report.sh render <report.json>}"
	local summary_file="${GITHUB_STEP_SUMMARY:-/dev/stdout}"
	local limit="${SECURITY_SCAN_RENDER_LIMIT:-50}"

	if [[ ! -f "$report" ]]; then
		echo "No trivy report at '$report'; skipping summary." >&2
		return 0
	fi

	write() { echo "$@" >>"$summary_file"; }

	table() {
		echo "| Severity | Class | ID | Target | Package | Installed | Fixed |"
		echo "|----------|-------|----|--------|---------|-----------|-------|"
		jq -r '.[] | "| \(.severity) | \(.class) | \(.id) | \(.target) | \(.pkg) | \(.installed) | \(.fixed) |"'
	}

	write "## Weekly Security Scan"
	write ""

	local baseline
	baseline=$(jq -r '.baseline' "$report")
	if [[ "$baseline" == "true" ]]; then
		write "Baseline established: $(jq -r "$(jq_lib)"'.current_counts | counts_line' "$report")"
	else
		write "| Severity | Current | Previous |"
		write "|----------|---------|----------|"
		jq -r '["CRITICAL","HIGH","MEDIUM","LOW","UNKNOWN","total"][] as $s |
			"| \($s) | \(.current_counts[$s] // 0) | \(.previous_counts[$s] // 0) |"' "$report" >>"$summary_file"
		write ""

		local new_count fixed_count
		new_count=$(jq -r '.new | length' "$report")
		fixed_count=$(jq -r '.fixed | length' "$report")

		write "### New findings ($new_count)"
		write ""
		if [[ "$new_count" -gt 0 ]]; then
			jq -c '.new' "$report" | table >>"$summary_file"
		else
			write "No new findings this week."
		fi
		write ""

		write "### Fixed findings ($fixed_count)"
		write ""
		if [[ "$fixed_count" -gt 0 ]]; then
			jq -c '.fixed' "$report" | table >>"$summary_file"
		else
			write "No fixed findings this week."
		fi
	fi
	write ""

	local total shown
	total=$(jq -r '.current_findings | length' "$report")
	shown=$((total < limit ? total : limit))

	write "### All current findings ($total)"
	write ""
	if [[ "$total" -gt 0 ]]; then
		write "<details><summary>Showing $shown of $total</summary>"
		write ""
		jq -c --argjson limit "$limit" '.current_findings[:$limit]' "$report" | table >>"$summary_file"
		if [[ "$total" -gt "$limit" ]]; then
			write ""
			write "...and $((total - limit)) more; see the security-scan-report.json artifact."
		fi
		write ""
		write "</details>"
	else
		write "No findings."
	fi
}

# Print the Slack message payload for the report to stdout
cmd_payload() {
	local report="${1:?Usage: security-scan-report.sh payload <report.json>}"
	local limit="${SECURITY_SCAN_NOTIFY_LIMIT:-10}"
	local repo="${REPO:-gruntwork-io/terragrunt}"
	local run_url="${GITHUB_SERVER_URL:-https://github.com}/${repo}/actions/runs/${GITHUB_RUN_ID:-0}"

	if [[ ! -f "$report" ]]; then
		echo "Error: trivy report '$report' not found" >&2
		exit 1
	fi

	# Header: one dated line per endpoint, matching the weekly coverage report.
	local cur_sha="${CURRENT_SHA:-}" cur_date="${CURRENT_DATE:-unknown}"
	local prev_sha="${PREVIOUS_SHA:-}" prev_date="${PREVIOUS_DATE:-unknown}"
	local header="*Weekly Security Scan: terragrunt*"
	if [[ -n "$cur_sha" && -n "$prev_sha" ]]; then
		header+=$'\n'"From: ${prev_date} ${prev_sha}"
		header+=$'\n'"To: ${cur_date} ${cur_sha}"
	elif [[ -n "$cur_sha" ]]; then
		header+=$'\n'"At: ${cur_date} ${cur_sha}"
	fi

	jq -n \
		--arg header "$header" \
		--arg run_url "$run_url" \
		--argjson limit "$limit" \
		--slurpfile rep "$report" \
		"$(jq_lib)"'
		def listed($items; $label): if ($items | length) > 0 then
			"\($label) (\($items | length)):\n"
			+ ([$items[:$limit][] | "  \(finding_line)"] | join("\n"))
			+ (if ($items | length) > $limit then "\n  ...and \(($items | length) - $limit) more" else "" end)
		else "" end;

		($rep[0]) as $r |

		(if $r.baseline then
			"Findings baseline: \($r.current_counts | counts_line)"
		else
			"Findings: \($r.current_counts | counts_line) (was \($r.previous_counts | counts_line))"
		end) as $totals |

		{
			new: listed($r.new; "New this week"),
			fixed: listed($r.fixed; "Fixed this week"),
			current: listed($r.current_findings; "Current findings")
		} as $lists |

		(if ($r.baseline | not) and $lists.new == "" then "No new findings this week." else "" end) as $none |

		{
			text: (
				($header + "\n\n")
				+ $totals
				+ (if $lists.new != "" then "\n\n" + $lists.new else "" end)
				+ (if $lists.fixed != "" then "\n\n" + $lists.fixed else "" end)
				+ (if $none != "" then "\n\n" + $none else "" end)
				+ (if $lists.current != "" then "\n\n" + $lists.current else "" end)
				+ "\n\n<\($run_url)|View workflow run>"
			)
		}'
}

# Post the report to Slack
cmd_notify() {
	local webhook="${SECURITY_SCAN_SLACK_WEBHOOK_URL:?Required environment variable SECURITY_SCAN_SLACK_WEBHOOK_URL}"
	local report="${1:?Usage: security-scan-report.sh notify <report.json>}"

	local payload
	payload=$(cmd_payload "$report")

	curl -sS --fail-with-body -X POST -H "Content-Type: application/json" -d "$payload" "$webhook"
	echo "Slack notification sent."
}

# Post a run-failure notice to Slack
cmd_notify_failure() {
	local webhook="${SECURITY_SCAN_SLACK_WEBHOOK_URL:?Required environment variable SECURITY_SCAN_SLACK_WEBHOOK_URL}"
	local repo="${REPO:-gruntwork-io/terragrunt}"
	local run_url="${GITHUB_SERVER_URL:-https://github.com}/${repo}/actions/runs/${GITHUB_RUN_ID:-0}"

	local payload
	payload=$(jq -n --arg run_url "$run_url" \
		'{text: "*Weekly Security Scan: terragrunt*\nRun failed before producing a report.\n\n<\($run_url)|View workflow run>"}')

	curl -sS --fail-with-body -X POST -H "Content-Type: application/json" -d "$payload" "$webhook"
	echo "Slack failure notification sent."
}

# Route the subcommand to its handler
main() {
	local cmd="${1:-}"
	shift || true
	case "$cmd" in
	scan) cmd_scan "$@" ;;
	summary) cmd_summary "$@" ;;
	compare) cmd_compare "$@" ;;
	render) cmd_render "$@" ;;
	payload) cmd_payload "$@" ;;
	notify) cmd_notify "$@" ;;
	notify-failure) cmd_notify_failure "$@" ;;
	"" | -h | --help | help) usage ;;
	*)
		echo "Unknown subcommand: $cmd" >&2
		usage >&2
		exit 1
		;;
	esac
}

main "$@"
