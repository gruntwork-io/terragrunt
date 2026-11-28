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
#                                    findings, minus those in report-suppressions.json (see
#                                    Suppressions below). A missing/empty previous summary
#                                    yields a baseline (current-only) report.
#   render <report.json>             Render the report as Markdown to $GITHUB_STEP_SUMMARY
#                                    (or stdout).
#   payload <report.json>            Print the Slack message payload JSON to stdout.
#   notify <report.json>             Post the report to Slack ($SECURITY_SCAN_SLACK_WEBHOOK_URL).
#   notify-failure                   Post a run-failure notice to Slack so a broken scan
#                                    is not a silent missed week.
#
# Suppressions: `compare` drops the findings listed in report-suppressions.json
# (next to this script) from the counts, new, fixed, and current findings, and
# lists them under "suppressed" instead so they stay visible in the report and
# the Slack message. Each entry needs an "id" (the CVE / GO / AVD / rule ID) and
# a "reason"; an optional "pkg" and "target" narrow the match (omitted = any),
# and an optional "expires" date (YYYY-MM-DD) makes the entry lapse. Entries
# that have expired or match no current finding are reported as stale.
#
# Env knobs — SECURITY_SCAN_* configure the scanner-agnostic reporting;
# TRIVY_* configure the scanner itself (intentionally shadowing trivy's native
# env config; the explicit flags passed by `scan` always win):
#   SECURITY_SCAN_RENDER_LIMIT  Max current findings rendered in the step summary (default: 50).
#   SECURITY_SCAN_NOTIFY_LIMIT  Max new/fixed findings listed in the Slack message (default: 10).
#   SECURITY_SCAN_SUPPRESSIONS_FILE  Suppressions config read by `compare` (default:
#                       report-suppressions.json next to this script).
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

# Shared jq defs: severity handling, the fields every finding class has in
# common, and the count/finding/list output formats.
jq_lib() {
	cat <<-'JQ'
		def sev: if IN("CRITICAL", "HIGH", "MEDIUM", "LOW") then . else "UNKNOWN" end;
		def sevrank: {"CRITICAL": 0, "HIGH": 1, "MEDIUM": 2, "LOW": 3}[.] // 4;
		def base($r): {
			id: "", pkg: "", installed: "", fixed: "", line: 0,
			severity: (.Severity | sev), target: ($r.Target // ""), title: (.Title // "")
		};
		def counts: {CRITICAL: 0, HIGH: 0, MEDIUM: 0, LOW: 0, UNKNOWN: 0}
			+ (group_by(.severity) | map({(.[0].severity): length}) | add // {})
			+ {total: length};
		def counts_line: . as $c |
			(["CRITICAL", "HIGH", "MEDIUM", "LOW"]
				| map(select(($c[.] // 0) > 0) | "\($c[.]) \(.)")
				| join(", ")) as $by_sev |
			"\($c.total // 0) total" + (if $by_sev != "" then " (\($by_sev))" else "" end);
		def loc: .target + (if (.line // 0) > 0 then ":\(.line)" else "" end);
		def finding_line:
			(if .severity != "UNKNOWN" then "[\(.severity)] " else "" end)
			+ (if .class == "vuln" then
				"\(.pkg) \(.installed)"
				+ (if .fixed != "" then " -> \(.fixed)" else "" end)
				+ " \(.id)"
			else
				.id + (if .title != "" then " \(.title)" else "" end)
			end)
			+ " (\(loc))";
		def today: now | strftime("%Y-%m-%d");
		def expired: (.expires // "9999-12-31") < today;
		def suppressed_by($entry):
			.id == $entry.id
			and ($entry.pkg == null or .pkg == $entry.pkg)
			and ($entry.target == null or .target == $entry.target);
		def suppression($entries): . as $f | first($entries[] | select(. as $e | $f | suppressed_by($e))) // null;
		def listed($items; $label; $limit): if ($items | length) == 0 then "" else
			"\($label) (\($items | length)):\n"
			+ ([$items[:$limit][] | "  \(finding_line)"] | join("\n"))
			+ (if ($items | length) > $limit
				then "\n  ...and \(($items | length) - $limit) more" else "" end)
		end;
	JQ
}

# Read the suppressions config as normalized JSON, failing on a malformed file
read_suppressions() {
	local file="${SECURITY_SCAN_SUPPRESSIONS_FILE:-${SELF%/*}/report-suppressions.json}"

	if [[ ! -f "$file" ]]; then
		echo '{"findings":[]}'
		return 0
	fi

	local normalized
	if ! normalized=$(jq '
		def field($name; $required):
			if has($name) then
				if (.[$name] | type) != "string" or .[$name] == ""
					then error("\($name) must be a non-empty string") else . end
			elif $required then error("\($name) is required")
			else . end;
		def entry($i):
			if type != "object" then error("findings[\($i)] must be an object") else . end
			| try (
				field("id"; true) | field("reason"; true)
				| field("pkg"; false) | field("target"; false) | field("expires"; false)
				| if has("expires") and (.expires | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}$") | not)
					then error("expires must be a YYYY-MM-DD date") else . end
				| (keys - ["id", "pkg", "target", "reason", "expires"]) as $unknown
				| if ($unknown | length) > 0 then error("unknown field(s) \($unknown | join(", "))") else . end
			) catch error("findings[\($i)]: \(.)");

		if type != "object" then error("expected a JSON object {\"findings\": [...]}") else . end
		| (keys - ["findings"]) as $unknown
		| if ($unknown | length) > 0 then error("unknown field(s) \($unknown | join(", ")); expected {\"findings\": [...]}") else . end
		| if ((.findings // []) | type) != "array" then error("findings must be an array") else . end
		| {findings: [(.findings // []) | to_entries[] | .key as $i | .value | entry($i)]}
	' "$file" 2>&1); then
		echo "Error: suppressions file '$file' is invalid: ${normalized#jq: error (at *): }" >&2
		exit 1
	fi

	echo "$normalized"
}

# Run `trivy fs` over a source tree and write the raw JSON report
cmd_scan() {
	local src="${1:?Usage: security-scan-report.sh scan <src-dir> <out.json> [trivy args...]}"
	local out="${2:?Usage: security-scan-report.sh scan <src-dir> <out.json> [trivy args...]}"
	shift 2

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
			(($r.Vulnerabilities // [])[] | base($r) + {
				class: "vuln",
				id: (.VulnerabilityID // ""),
				pkg: (.PkgName // ""),
				installed: (.InstalledVersion // ""),
				fixed: (.FixedVersion // "")
			}),
			(($r.Misconfigurations // [])[] | select((.Status // "FAIL") == "FAIL") | base($r) + {
				class: "misconfig",
				id: (.AVDID // .ID // ""),
				line: (.CauseMetadata.StartLine // 0)
			}),
			(($r.Secrets // [])[] | base($r) + {
				class: "secret",
				id: (.RuleID // ""),
				line: (.StartLine // 0)
			})
		]
		| map(. + {key: "\(.class)|\(.target)|\(.pkg)|\(.id)"})
		| unique_by(.key)
		| sort_by([(.severity | sevrank), .key])
		| {commit: $commit, timestamp: $timestamp, counts: counts, findings: .}' "$input" >"$output"

	echo "Summary: $output ($(jq -r '.counts.total' "$output") findings)"
}

# Diff two findings summaries into a new/fixed report, minus suppressed findings
cmd_compare() {
	local current="${1:?Usage: security-scan-report.sh compare <current.json> <previous.json> [output.json]}"
	local previous="${2:?Usage: security-scan-report.sh compare <current.json> <previous.json> [output.json]}"
	local output="${3:-security-scan-report.json}"

	if [[ ! -f "$current" ]]; then
		echo "Error: current summary '$current' not found" >&2
		exit 1
	fi

	local suppressions
	suppressions=$(read_suppressions)

	# An empty previous slurps to [], so $p is null and the report is a baseline.
	if [[ ! -s "$previous" ]]; then
		echo "No previous scan data found: establishing baseline."
		previous=/dev/null
	fi

	mkdir -p "$(dirname "$output")"

	# Suppressions apply to both sides, so a suppressed finding is neither new nor
	# fixed; the current ones are kept under "suppressed" with their reason.
	jq -n \
		--slurpfile cur "$current" \
		--slurpfile prev "$previous" \
		--argjson suppressions "$suppressions" \
		"$(jq_lib)"'
		def visible($active): map(select(suppression($active) == null));

		($cur[0]) as $c |
		($prev[0]) as $p |
		[$suppressions.findings[] | select(expired | not)] as $active |
		($c.findings | visible($active)) as $cur_findings |
		(($p.findings // []) | visible($active)) as $prev_findings |
		INDEX($cur_findings[]; .key) as $ckeys |
		INDEX($prev_findings[]; .key) as $pkeys |
		{
			baseline: ($p == null),
			current_counts: ($cur_findings | counts),
			previous_counts: (if $p == null then null else ($prev_findings | counts) end),
			new: (if $p == null then [] else [$cur_findings[] | select($pkeys[.key] | not)] end),
			fixed: (if $p == null then [] else [$prev_findings[] | select($ckeys[.key] | not)] end),
			unchanged: (if $p == null then null else ([$cur_findings[] | select($pkeys[.key])] | length) end),
			current_findings: $cur_findings,
			suppressed: [$c.findings[] | suppression($active) as $s | select($s != null) | . + {reason: $s.reason}],
			stale_suppressions: [$suppressions.findings[]
				| . + {state: (if expired then "expired" else "unmatched" end)}
				| select(.state == "expired" or (. as $e | any($c.findings[]; suppressed_by($e)) | not))]
		}' >"$output"

	echo "Report: $output ($(jq -r '"\(.new | length) new, \(.fixed | length) fixed, \(.suppressed | length) suppressed"' "$output"))"
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
		echo "| Severity | Class | ID | Location | Package | Installed | Fixed |"
		echo "|----------|-------|----|----------|---------|-----------|-------|"
		jq -r "$(jq_lib)"'.[] | "| \(.severity) | \(.class) | \(.id) | \(loc) | \(.pkg) | \(.installed) | \(.fixed) |"'
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

	# Older reports predate suppressions, hence the `// []` fallbacks.
	local suppressed_count stale_count
	suppressed_count=$(jq -r '.suppressed // [] | length' "$report")
	stale_count=$(jq -r '.stale_suppressions // [] | length' "$report")

	if [[ "$suppressed_count" -gt 0 ]]; then
		write ""
		write "### Suppressed findings ($suppressed_count)"
		write ""
		write "| Severity | Class | ID | Location | Package | Installed | Reason |"
		write "|----------|-------|----|----------|---------|-----------|--------|"
		jq -r "$(jq_lib)"'.suppressed[] | "| \(.severity) | \(.class) | \(.id) | \(loc) | \(.pkg) | \(.installed) | \(.reason) |"' "$report" >>"$summary_file"
	fi

	if [[ "$stale_count" -gt 0 ]]; then
		write ""
		write "### Stale suppressions ($stale_count)"
		write ""
		write "These report-suppressions.json entries no longer suppress anything; remove or renew them:"
		write ""
		jq -r '.stale_suppressions[] | "- \(.id)"
			+ (if .pkg then " \(.pkg)" else "" end)
			+ (if .target then " (\(.target))" else "" end)
			+ " - \(.state)"
			+ (if .expires then " \(.expires)" else "" end)' "$report" >>"$summary_file"
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
		($rep[0]) as $r |

		(if $r.baseline then
			"Findings baseline: \($r.current_counts | counts_line)"
		else
			"Findings: \($r.current_counts | counts_line) (was \($r.previous_counts | counts_line))"
		end) as $totals |

		listed($r.new; "New this week"; $limit) as $new |
		(if ($r.baseline | not) and $new == "" then "No new findings this week." else "" end) as $none |
		listed($r.suppressed // []; "Suppressed"; $limit) as $suppressed |
		(($r.stale_suppressions // []) | if length == 0 then "" else
			"Stale suppressions (\(length)): " + (map("\(.id) (\(.state))") | join(", "))
		end) as $stale |

		{text: ([
			$header,
			$totals,
			$new,
			listed($r.fixed; "Fixed this week"; $limit),
			$none,
			listed($r.current_findings; "Current findings"; $limit),
			$suppressed,
			$stale,
			"<\($run_url)|View workflow run>"
		] | map(select(. != "")) | join("\n\n"))}'
}

# Post the report to Slack
cmd_notify() {
	local webhook="${SECURITY_SCAN_SLACK_WEBHOOK_URL:?Required environment variable SECURITY_SCAN_SLACK_WEBHOOK_URL}"
	local report="${1:?Usage: security-scan-report.sh notify <report.json>}"

	local payload
	payload=$(cmd_payload "$report")

	curl -sS --fail-with-body --connect-timeout 10 --max-time 30 \
		-X POST -H "Content-Type: application/json" -d "$payload" "$webhook"
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

	curl -sS --fail-with-body --connect-timeout 10 --max-time 30 \
		-X POST -H "Content-Type: application/json" -d "$payload" "$webhook"
	echo "Slack failure notification sent."
}

# Fail before any work when a subcommand's tools are missing
require_tools() {
	local tool missing=()
	for tool in "$@"; do
		command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
	done
	if [[ ${#missing[@]} -gt 0 ]]; then
		echo "Error: not found on PATH: ${missing[*]}. Install them (e.g. aquasecurity/setup-trivy for trivy) first." >&2
		exit 1
	fi
}

# Route the subcommand to its handler, after checking its pre-reqs
main() {
	local cmd="${1:-}"
	shift || true
	case "$cmd" in
	scan) require_tools jq trivy && cmd_scan "$@" ;;
	summary) require_tools jq && cmd_summary "$@" ;;
	compare) require_tools jq && cmd_compare "$@" ;;
	render) require_tools jq && cmd_render "$@" ;;
	payload) require_tools jq && cmd_payload "$@" ;;
	notify) require_tools jq curl && cmd_notify "$@" ;;
	notify-failure) require_tools jq curl && cmd_notify_failure "$@" ;;
	"" | -h | --help | help) usage ;;
	*)
		echo "Unknown subcommand: $cmd" >&2
		usage >&2
		exit 1
		;;
	esac
}

main "$@"
