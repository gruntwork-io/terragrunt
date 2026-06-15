#!/usr/bin/env bash
set -euo pipefail

# Single entrypoint for test coverage + runtime reporting.
#
# Subcommands:
#   run <out-dir> [packages...]      Run the suite with -json. Writes coverage.out,
#                                    test-events.ndjson and result.xml into <out-dir>.
#                                    Packages default to ./... (the full suite).
#   annotate <events.ndjson>         Emit GitHub ::error annotations (file, line,
#                                    failure output) for failed tests in an events file.
#   collect <out-dir> [packages...]  run + summary + timing in one call (tolerates
#                                    test failures); produces both summaries.
#   summary <cover.out> <out.json>   Roll a cover profile into per-package coverage
#                                    JSON + an HTML report. Args default to
#                                    coverage.out / coverage-summary.json.
#   timing <events.ndjson> <out.json>  Roll `go test -json` events into per-package /
#                                    per-test wall-time JSON.
#   compare-coverage <cur> <prev> [out]  Diff two coverage summaries -> report JSON + HTML.
#   compare-timing <cur> <prev> [out]    Diff two timing summaries -> report JSON.
#   render <cov-report> <timing-report>  Render the combined Markdown report to
#                                    $GITHUB_STEP_SUMMARY (or stdout).
#   notify <cov-report> <timing-report>  Post the combined report to Slack.
#   local                            Clone a fresh copy and reproduce the whole weekly
#                                    report locally (see `local --help`).
#
# Pass a missing-previous path as /nonexistent to the compare subcommands to get a
# baseline (current-only) report.

SELF="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
COVERAGE_CHANGE_THRESHOLD="${COVERAGE_CHANGE_THRESHOLD:-3}"

usage() {
	sed -n '/^# Single entrypoint/,/^# baseline (current-only) report\.$/p' "$SELF"
}

now_utc() { date -u +%Y-%m-%dT%H:%M:%SZ; }
this_commit() { echo "${GITHUB_SHA:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}"; }
this_ref() { echo "${GITHUB_REF:-$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)}"; }

# Run the suite (-json) and emit coverage.out, test-events.ndjson and result.xml
cmd_run() {
	local out="${1:?Usage: coverage-report.sh run <out-dir> [packages...]}"
	shift
	local pkgs=("$@")
	[[ ${#pkgs[@]} -eq 0 ]] && pkgs=("./...")

	mkdir -p "$out"
	local events="$out/test-events.ndjson" cover="$out/coverage.out" junit="$out/result.xml"

	# github-actions format emits ::error file=...,line=... annotations and
	# re-prints failed test output in the end-of-run summary, so failures are
	# readable straight from the job log.
	local format="${GOTESTSUM_FORMAT:-pkgname}"
	[[ -n "${GITHUB_ACTIONS:-}" ]] && format="${GOTESTSUM_FORMAT:-github-actions}"

	set +e
	gotestsum \
		--format "$format" \
		--jsonfile "$events" \
		--junitfile "$junit" \
		-- -coverprofile="$cover" -covermode=atomic -timeout "${TEST_TIMEOUT:-45m}" "${pkgs[@]}"
	local status=$?
	set -e

	echo "go test exit status: $status"
	echo "Events: $events ($(wc -l <"$events") lines)"
	echo "Cover:  $cover"
	echo "JUnit:  $junit"
	return "$status"
}

# Emit GitHub ::error annotations for failed tests from go test -json events.
# Only tests whose output carries a `foo_test.go:NN:` location are annotated;
# GitHub shows at most 10 error annotations per step, the rest stay in the log.
cmd_annotate() {
	local events="${1:?Usage: coverage-report.sh annotate <events.ndjson>}"
	if [[ ! -f "$events" ]]; then
		echo "Error: events file '$events' not found" >&2
		exit 1
	fi

	local module
	module="$(go list -m)"

	jq -rn --raw-input --arg module "$module" '
		def esc_data: gsub("%"; "%25") | gsub("\r"; "%0D") | gsub("\n"; "%0A");
		def esc_prop: esc_data | gsub(":"; "%3A") | gsub(","; "%2C");

		reduce (
			inputs
			| select(length > 0)
			| fromjson?
			| select((.Test // "") != "")
		) as $e (
			{out: {}, failed: []};
			($e.Package + "|" + $e.Test) as $k
			| if $e.Action == "output" then .out[$k] += [$e.Output]
			elif $e.Action == "fail" then .failed += [$k]
			else . end
		)
		| .out as $out
		| .failed[]
		| split("|") as [$pkg, $test]
		| ($pkg | ltrimstr($module) | ltrimstr("/")) as $dir
		| ($out[$pkg + "|" + $test] // []) as $lines
		| ($lines | map(capture("^\\s+(?<file>[^\\s:]+_test\\.go):(?<line>[0-9]+): ")) | first) as $loc
		| select($loc != null)
		| (if $dir == "" then $loc.file else $dir + "/" + $loc.file end) as $file
		| ($lines | map(select(test("^(===|--- )") | not)) | join("") | .[0:2500]) as $msg
		| "::error file=\($file | esc_prop),line=\($loc.line),title=\($test | esc_prop) (\($pkg | esc_prop))::\($msg | esc_data)"
	' "$events"
}

# Roll a cover profile into per-package coverage JSON plus an HTML report
cmd_summary() {
	local cover="${1:-coverage.out}" output="${2:-coverage-summary.json}"
	if [[ ! -f "$cover" ]]; then
		echo "Error: coverage file '$cover' not found" >&2
		exit 1
	fi

	local total
	total=$(go tool cover -func="$cover" | grep '^total:' | awk '{print $NF}' | tr -d '%')

	# Per-package average coverage (go tool cover reports per-function); awk emits
	# TSV, jq handles JSON escaping.
	local packages_json
	packages_json=$(go tool cover -func="$cover" |
		grep -v '^total:' |
		awk -F'\t+' '{
			split($1, parts, ":")
			file = parts[1]
			n = split(file, segs, "/")
			pkg = ""
			for (i = 1; i < n; i++) {
				if (pkg != "") pkg = pkg "/"
				pkg = pkg segs[i]
			}
			pct = $NF
			gsub(/%/, "", pct)
			sum[pkg] += pct
			count[pkg]++
		}
		END {
			for (p in sum) printf "%s\t%.1f\n", p, sum[p]/count[p]
		}' | jq -Rs 'split("\n") | map(select(length > 0) | split("\t") | {(.[0]): (.[1] | tonumber)}) | add // {}')

	jq -n \
		--arg total "$total" \
		--arg ts "$(now_utc)" \
		--arg commit "$(this_commit)" \
		--arg ref "$(this_ref)" \
		--argjson pkgs "$packages_json" \
		'{total_pct: ($total | tonumber), timestamp: $ts, commit: $commit, ref: $ref, packages: $pkgs}' >"$output"

	local html_output="${output%.json}.html"
	go tool cover -html="$cover" -o "$html_output"

	echo "=== Coverage: ${total}% ==="
	echo ""
	jq -r '.packages | to_entries | sort_by(.value) | .[] | "\(.value)%\t\(.key)"' "$output" | column -t -s $'\t'
	echo ""
	echo "Written to $output"
	echo "HTML report: $html_output"
}

# Aggregate go test -json events into per-package and per-test wall times
# Streamed in one jq pass; passing the full aggregate via --argjson hits ARG_MAX
cmd_timing() {
	local events="${1:?Usage: coverage-report.sh timing <events.ndjson> <output.json>}"
	local output="${2:?Usage: coverage-report.sh timing <events.ndjson> <output.json>}"
	if [[ ! -f "$events" ]]; then
		echo "Error: events file '$events' not found" >&2
		exit 1
	fi

	jq -n --raw-input \
		--arg ts "$(now_utc)" \
		--arg commit "$(this_commit)" \
		--arg ref "$(this_ref)" '
		reduce (
			inputs
			| select(length > 0)
			| fromjson?
			| select(.Action == "pass" or .Action == "fail" or .Action == "skip")
		) as $e (
			{packages: {}};
			if ($e.Test // "") == "" then
				.packages[$e.Package] = (
					(.packages[$e.Package] // {wall_sec: 0, tests: {}})
					| .wall_sec = ($e.Elapsed // 0)
				)
			else
				.packages[$e.Package] = (
					(.packages[$e.Package] // {wall_sec: 0, tests: {}})
					| .tests[$e.Test] = ($e.Elapsed // 0)
				)
			end
		)
		| .total_sec = ([.packages[].wall_sec] | add // 0)
		| . + {generated_at: $ts, commit: $commit, ref: $ref}
	' "$events" >"$output"

	echo "=== Timing summary ==="
	local total pkgs
	total=$(jq -r '.total_sec' "$output")
	pkgs=$(jq -r '.packages | length' "$output")
	printf "Total: %.1fs across %d packages\n" "$total" "$pkgs"
	echo ""
	echo "Top 5 slowest packages:"
	jq -r '.packages | to_entries | sort_by(-.value.wall_sec) | .[:5][] | "  \(.value.wall_sec)s\t\(.key)"' "$output"
	echo ""
	echo "Written to $output"
}

# Diff two coverage summaries into a report JSON plus HTML
cmd_compare_coverage() {
	local current="${1:?Usage: coverage-report.sh compare-coverage <current.json> <previous.json> [output.json]}"
	local previous="${2:?Usage: coverage-report.sh compare-coverage <current.json> <previous.json> [output.json]}"
	local output="${3:-comparison-report.json}"
	local html_output="${output%.json}.html"

	if [[ ! -f "$current" ]]; then
		echo "Error: current coverage file '$current' not found" >&2
		exit 1
	fi

	if [[ ! -s "$previous" ]]; then
		echo "No previous coverage data found: establishing baseline."
		jq -n --argjson curr "$(cat "$current")" '{baseline: true, current_total: $curr.total_pct, previous_total: null, total_delta: null, top_drops: [], top_gains: []}' >"$output"
		return 0
	fi

	local curr_total prev_total total_delta
	curr_total=$(jq -r '.total_pct' "$current")
	prev_total=$(jq -r '.total_pct' "$previous")
	total_delta=$(bc -l <<<"$curr_total - $prev_total")

	local curr_ref prev_ref curr_commit prev_commit curr_ts prev_ts
	curr_ref=$(jq -r '.ref // "head"' "$current")
	prev_ref=$(jq -r '.ref // "base"' "$previous")
	curr_commit=$(jq -r '.commit // "unknown"' "$current")
	prev_commit=$(jq -r '.commit // "unknown"' "$previous")
	curr_ts=$(jq -r '.timestamp // ""' "$current")
	prev_ts=$(jq -r '.timestamp // ""' "$previous")

	local package_comparison top_drops top_gains
	package_comparison=$(jq -n \
		--argjson curr "$(jq '.packages' "$current")" \
		--argjson prev "$(jq '.packages' "$previous")" \
		'[$prev | keys[], ($curr | keys[])] | unique | map(
			. as $pkg |
			{
				package: $pkg,
				current: ($curr[$pkg] // null),
				previous: ($prev[$pkg] // null),
				delta: (if ($curr[$pkg] != null) and ($prev[$pkg] != null)
					then (($curr[$pkg] - $prev[$pkg]) * 10 | round / 10)
					else null end)
			}
		)')

	top_drops=$(jq '[map(select(.delta != null and .delta < 0)) | sort_by(.delta) | .[:10] | .[]]' <<<"$package_comparison")
	top_gains=$(jq '[map(select(.delta != null and .delta > 0)) | sort_by(-.delta) | .[:5] | .[]]' <<<"$package_comparison")

	jq -n \
		--argjson curr_total "$curr_total" \
		--argjson prev_total "$prev_total" \
		--argjson total_delta "$total_delta" \
		--argjson threshold "${COVERAGE_CHANGE_THRESHOLD}" \
		--argjson top_drops "$top_drops" \
		--argjson top_gains "$top_gains" \
		'{
			baseline: false,
			current_total: $curr_total,
			previous_total: $prev_total,
			total_delta: ($total_delta * 10 | round / 10),
			coverage_threshold: $threshold,
			significant_change: ((($total_delta * 10 | round / 10) | fabs) >= $threshold),
			top_drops: $top_drops,
			top_gains: $top_gains
		}' >"$output"

	{
		cat <<-HEADER
			<!DOCTYPE html>
			<html><head>
			<meta charset="utf-8">
			<title>Coverage Comparison</title>
			<style>
			body { font-family: monospace; margin: 2em; }
			table { border-collapse: collapse; width: 100%; }
			th, td { border: 1px solid #ccc; padding: 6px 12px; text-align: right; }
			th { background: #f5f5f5; }
			td:first-child, th:first-child { text-align: left; }
			tr.total { font-weight: bold; border-top: 2px solid #333; }
			tr.new td, tr.removed td { font-style: italic; }
			</style>
			</head><body>
			<h2>Coverage Comparison</h2>
			<table>
			<tr><td>Base</td><td>${prev_ref} (${prev_commit:0:12})</td><td>${prev_ts}</td></tr>
			<tr><td>Head</td><td>${curr_ref} (${curr_commit:0:12})</td><td>${curr_ts}</td></tr>
			</table>
			<br>
			<table>
			<tr><th>Package</th><th>Base %</th><th>Head %</th><th>Delta</th></tr>
		HEADER

		printf '<tr class="total"><td>TOTAL</td><td>%.1f%%</td><td>%.1f%%</td><td>%+.1f%%</td></tr>\n' "$prev_total" "$curr_total" "$total_delta"

		jq -r 'sort_by(.package) | .[] |
			if .previous == null then
				"<tr class=\"new\"><td>\(.package)</td><td>n/a</td><td>\(.current)%</td><td>new</td></tr>"
			elif .current == null then
				"<tr class=\"removed\"><td>\(.package)</td><td>\(.previous)%</td><td>n/a</td><td>removed</td></tr>"
			else
				"<tr><td>\(.package)</td><td>\(.previous)%</td><td>\(.current)%</td><td>\(.delta)%</td></tr>"
			end' <<<"$package_comparison"

		cat <<-FOOTER
			</table>
			</body></html>
		FOOTER
	} >"$html_output"

	echo "=== Coverage Comparison ==="
	printf "Total: %.1f%% (was %.1f%%, delta: %+.1f%%)\n" "$curr_total" "$prev_total" "$total_delta"
	echo ""

	if [[ $(jq 'length' <<<"$top_drops") -gt 0 ]]; then
		echo "Top drops:"
		jq -r '.[] | "  \(.package): \(.previous)% -> \(.current)% (\(.delta)%)"' <<<"$top_drops"
		echo ""
	fi

	if [[ $(jq 'length' <<<"$top_gains") -gt 0 ]]; then
		echo "Top gains:"
		jq -r '.[] | "  \(.package): \(.previous)% -> \(.current)% (+\(.delta)%)"' <<<"$top_gains"
		echo ""
	fi

	echo "HTML report: $html_output"
}

# Diff two timing summaries into a report JSON
cmd_compare_timing() {
	local current="${1:?Usage: coverage-report.sh compare-timing <current.json> <previous.json> [output.json]}"
	local previous="${2:?Usage: coverage-report.sh compare-timing <current.json> <previous.json> [output.json]}"
	local output="${3:-timing-comparison.json}"

	if [[ ! -f "$current" ]]; then
		echo "Error: current timing file '$current' not found" >&2
		exit 1
	fi

	if [[ ! -s "$previous" ]]; then
		echo "No previous timing data found - emitting current-only report."
		jq '{
			baseline: true,
			current_total_sec: ((.total_sec * 10 | round) / 10),
			previous_total_sec: null,
			total_delta_sec: null,
			slow_packages: (
				.packages | to_entries | sort_by(-.value.wall_sec) | .[:5] | map({
					package: .key,
					current_sec: .value.wall_sec,
					previous_sec: null,
					delta_sec: null,
					top_tests: (
						.value.tests | to_entries | sort_by(-.value) | .[:5] | map({
							name: .key,
							current_sec: .value,
							previous_sec: null,
							delta_sec: null
						})
					)
				})
			),
			top_regressions: []
		}' "$current" >"$output"
		return 0
	fi

	jq -n \
		--slurpfile curr "$current" \
		--slurpfile prev "$previous" \
		'
		($curr[0]) as $c |
		($prev[0]) as $p |
		($c.packages) as $cp |
		($p.packages) as $pp |

		([$cp | keys[], ($pp | keys[])] | unique) as $all_pkgs |

		($all_pkgs | map(
			. as $pkg |
			($cp[$pkg].wall_sec // null) as $cs |
			($pp[$pkg].wall_sec // null) as $ps |
			{
				package: $pkg,
				current_sec: $cs,
				previous_sec: $ps,
				delta_sec: (
					if ($cs != null) and ($ps != null)
					then (($cs - $ps) * 10 | round / 10)
					else null end
				),
				current_tests: ($cp[$pkg].tests // {}),
				previous_tests: ($pp[$pkg].tests // {})
			}
		)) as $pkg_diff |

		([$pkg_diff[] | select(.current_sec != null)] | sort_by(-.current_sec) | .[:5] | map(
			. as $row |
			{
				package: $row.package,
				current_sec: $row.current_sec,
				previous_sec: $row.previous_sec,
				delta_sec: $row.delta_sec,
				top_tests: (
					$row.current_tests | to_entries | sort_by(-.value) | .[:5] | map(
						. as $t |
						{
							name: $t.key,
							current_sec: $t.value,
							previous_sec: ($row.previous_tests[$t.key] // null),
							delta_sec: (
								if ($row.previous_tests[$t.key] // null) != null
								then (($t.value - $row.previous_tests[$t.key]) * 100 | round / 100)
								else null end
							)
						}
					)
				)
			}
		)) as $slow_packages |

		([$pkg_diff[] | select(.delta_sec != null and .delta_sec > 0)]
		 | sort_by(-.delta_sec) | .[:5] | map({
			package: .package,
			current_sec: .current_sec,
			previous_sec: .previous_sec,
			delta_sec: .delta_sec
		})) as $top_regressions |

		{
			baseline: false,
			current_total_sec: ((($c.total_sec // 0) * 10 | round) / 10),
			previous_total_sec: ((($p.total_sec // 0) * 10 | round) / 10),
			total_delta_sec: ((($c.total_sec // 0) - ($p.total_sec // 0)) * 10 | round / 10),
			slow_packages: $slow_packages,
			top_regressions: $top_regressions
		}
		' >"$output"

	echo "=== Timing Comparison ==="
	printf "Total: %.1fs (was %.1fs, delta: %+.1fs)\n" \
		"$(jq -r '.current_total_sec' "$output")" \
		"$(jq -r '.previous_total_sec' "$output")" \
		"$(jq -r '.total_delta_sec' "$output")"
	echo ""
	echo "Top 5 slowest packages:"
	jq -r '.slow_packages[] | "  \(.current_sec)s (\(.delta_sec // "n/a")s)\t\(.package)"' "$output"
	echo ""
	if [[ $(jq '.top_regressions | length' "$output") -gt 0 ]]; then
		echo "Top 5 wall-time regressions:"
		jq -r '.top_regressions[] | "  +\(.delta_sec)s (was \(.previous_sec)s)\t\(.package)"' "$output"
	fi
	echo ""
	echo "Written to $output"
}

# Render the combined coverage and timing report as Markdown
cmd_render() {
	local cov="${1:?Usage: coverage-report.sh render <coverage-report.json> <timing-report.json>}"
	local tim="${2:?Usage: coverage-report.sh render <coverage-report.json> <timing-report.json>}"
	local summary_file="${GITHUB_STEP_SUMMARY:-/dev/stdout}"

	if [[ ! -f "$cov" ]]; then
		echo "No coverage comparison report at '$cov'; skipping summary." >&2
		return 0
	fi

	write() { echo "$@" >>"$summary_file"; }

	write "## Weekly Coverage + Timing Report"
	write ""

	local cov_baseline
	cov_baseline=$(jq -r '.baseline' "$cov")
	if [[ "$cov_baseline" == "true" ]]; then
		write "### Coverage"
		write ""
		write "Baseline established. Total: $(jq -r '.current_total' "$cov")%"
	else
		write "### Coverage"
		write ""
		write "| Metric | Value |"
		write "|--------|-------|"
		write "| Current | $(jq -r '.current_total' "$cov")% |"
		write "| Previous | $(jq -r '.previous_total' "$cov")% |"
		write "| Delta | $(jq -r '.total_delta' "$cov")% |"
		write ""

		local drops gains
		drops=$(jq -r '.top_drops[:5][] | "| \(.package) | \(.previous)% | \(.current)% | \(.delta)% |"' "$cov" 2>/dev/null || true)
		if [[ -n "$drops" ]]; then
			write "#### Top 5 coverage drops"
			write "| Package | Previous | Current | Delta |"
			write "|---------|----------|---------|-------|"
			write "$drops"
			write ""
		fi

		gains=$(jq -r '.top_gains[:5][] | "| \(.package) | \(.previous)% | \(.current)% | +\(.delta)% |"' "$cov" 2>/dev/null || true)
		if [[ -n "$gains" ]]; then
			write "#### Top 5 coverage gains"
			write "| Package | Previous | Current | Delta |"
			write "|---------|----------|---------|-------|"
			write "$gains"
			write ""
		fi
	fi

	if [[ ! -f "$tim" ]]; then
		write "### Test Runtime"
		write ""
		write "No timing report available."
		return 0
	fi

	local tim_baseline
	tim_baseline=$(jq -r '.baseline' "$tim")

	write "### Test Runtime"
	write ""
	if [[ "$tim_baseline" == "true" ]]; then
		printf "| Metric | Value |\n| --- | --- |\n| Current total | %ss |\n| Previous total | n/a (baseline) |\n\n" \
			"$(jq -r '.current_total_sec' "$tim")" >>"$summary_file"
	else
		{
			printf "| Metric | Value |\n| --- | --- |\n"
			printf "| Current total | %ss |\n" "$(jq -r '.current_total_sec' "$tim")"
			printf "| Previous total | %ss |\n" "$(jq -r '.previous_total_sec' "$tim")"
			printf "| Delta | %+ss |\n\n" "$(jq -r '.total_delta_sec' "$tim")"
		} >>"$summary_file"
	fi

	write "#### Top 5 slowest packages"
	write "| Package | Current | Previous | Delta |"
	write "|---------|---------|----------|-------|"
	jq -r '.slow_packages[] |
		"| \(.package) | \(.current_sec)s | \(if .previous_sec == null then "n/a" else "\(.previous_sec)s" end) | \(if .delta_sec == null then "n/a" else "\(.delta_sec)s" end) |"' "$tim" >>"$summary_file"
	write ""

	write "#### Slowest tests per slow package"
	local slow_pkgs
	slow_pkgs=$(jq -c '.slow_packages[]' "$tim")
	while IFS= read -r row; do
		[[ -z "$row" ]] && continue
		local pkg
		pkg=$(jq -r '.package' <<<"$row")
		write ""
		write "<details><summary><code>${pkg}</code></summary>"
		write ""
		write "| Test | Current | Previous | Delta |"
		write "|------|---------|----------|-------|"
		jq -r '.top_tests[] |
			"| `\(.name)` | \(.current_sec)s | \(if .previous_sec == null then "n/a" else "\(.previous_sec)s" end) | \(if .delta_sec == null then "n/a" else "\(.delta_sec)s" end) |"' <<<"$row" >>"$summary_file"
		write ""
		write "</details>"
	done <<<"$slow_pkgs"

	local regressions
	regressions=$(jq -r '.top_regressions[] | "| \(.package) | \(.previous_sec)s | \(.current_sec)s | +\(.delta_sec)s |"' "$tim" 2>/dev/null || true)
	if [[ -n "$regressions" ]]; then
		write ""
		write "#### Top 5 wall-time regressions (week over week)"
		write "| Package | Previous | Current | Delta |"
		write "|---------|----------|---------|-------|"
		write "$regressions"
	fi
}

# Post the combined coverage and timing report to Slack
cmd_notify() {
	local webhook="${COVERAGE_SLACK_WEBHOOK_URL:?Required environment variable COVERAGE_SLACK_WEBHOOK_URL}"
	local cov="${1:?Usage: coverage-report.sh notify <coverage-report.json> <timing-report.json>}"
	local tim="${2:?Usage: coverage-report.sh notify <coverage-report.json> <timing-report.json>}"
	local tag="${TAG_NAME:-weekly}"
	local repo="${REPO:-gruntwork-io/terragrunt}"
	local run_url="${GITHUB_SERVER_URL:-https://github.com}/${repo}/actions/runs/${GITHUB_RUN_ID:-0}"

	# Header: one dated line per endpoint rather than a single prev...current line.
	local cur_sha="${CURRENT_SHA:-}" cur_date="${CURRENT_DATE:-unknown}"
	local prev_sha="${PREVIOUS_SHA:-}" prev_date="${PREVIOUS_DATE:-unknown}"
	local header="*Weekly Coverage + Runtime: terragrunt*"
	if [[ -n "$cur_sha" && -n "$prev_sha" ]]; then
		header+=$'\n'"From: ${prev_date} ${prev_sha}"
		header+=$'\n'"To: ${cur_date} ${cur_sha}"
	elif [[ -n "$cur_sha" ]]; then
		header+=$'\n'"At: ${cur_date} ${cur_sha}"
	else
		header="*Weekly Coverage + Runtime: terragrunt ${tag}*"
	fi

	if [[ ! -f "$cov" ]]; then
		echo "Error: coverage report '$cov' not found" >&2
		exit 1
	fi
	if [[ ! -f "$tim" ]]; then
		echo "Error: timing report '$tim' not found" >&2
		exit 1
	fi

	local payload
	payload=$(jq -n \
		--arg header "$header" \
		--arg run_url "$run_url" \
		--slurpfile cov "$cov" \
		--slurpfile tim "$tim" '
		($cov[0]) as $c |
		($tim[0]) as $t |

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
				($header + "\n\n")
				+ $cov_block
				+ "\n\n" + $rt_line
				+ (if $slow_pkgs != "" then "\n\n" + $slow_pkgs else "" end)
				+ (if $regressions != "" then "\n\n" + $regressions else "" end)
				+ "\n\n<\($run_url)|View workflow run>"
			)
		}')

	curl -sf -X POST -H "Content-Type: application/json" -d "$payload" "$webhook"
	echo "Slack notification sent."
}

# Clone a fresh copy and reproduce the whole weekly report locally
cmd_local() {
	if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
		cat <<-'HELP'
			Usage: coverage-report.sh local

			Env knobs (all optional):
			  REPO_URL      Repo to clone. Default: canonical GitHub URL. Use a local
			                path for a fast, offline run.
			  REF           Branch/tag/SHA checked out as "current". Default: main.
			  WINDOW_DAYS   Days back for the "previous" baseline. Default: 7.
			  PKGS          Package set to test. Default: ./... (full suite).
			                Set e.g. "./pkg/log/..." for a fast harness smoke run.
			  TEST_TIMEOUT  go test -timeout value. Default: 45m.
			  WORKDIR       Scratch dir. Default: a fresh mktemp dir (kept).
		HELP
		return 0
	fi

	local repo_url="${REPO_URL:-https://github.com/gruntwork-io/terragrunt.git}"
	local ref="${REF:-main}"
	local window_days="${WINDOW_DAYS:-7}"
	local pkgs_str="${PKGS:-./...}"
	local workdir="${WORKDIR:-$(mktemp -d)}"
	local src="$workdir/src" prevsrc="$workdir/prev"
	local out_cur="$workdir/current" out_prev="$workdir/previous"

	read -r -a pkgs <<<"$pkgs_str"

	echo "=== Config ==="
	printf 'repo=%s ref=%s window_days=%s pkgs=%s workdir=%s\n' \
		"$repo_url" "$ref" "$window_days" "$pkgs_str" "$workdir"

	echo "=== Clone $ref ==="
	git clone --no-single-branch "$repo_url" "$src"
	git -C "$src" checkout --detach "$ref"
	# Prefer the cloned copy (faithful to the ref); fall back to this script when the
	# ref predates it, so the report can be exercised before it is committed.
	local report="$src/.github/scripts/coverage/coverage-report.sh"
	if [[ ! -x "$report" ]]; then
		echo "Note: $ref has no coverage-report.sh; using this working-tree copy instead."
		report="$SELF"
	fi

	local cur prev=""
	cur="$(git -C "$src" rev-parse HEAD)"
	local max_days=$((window_days * 2))
	local d candidate
	for d in $(seq "$window_days" "$max_days"); do
		candidate="$(git -C "$src" rev-list -1 --before="${d} days ago" "$cur" || true)"
		if [[ -n "$candidate" && "$candidate" != "$cur" ]]; then
			prev="$candidate"
			break
		fi
	done
	echo "current:  $cur"
	echo "previous: ${prev:-<none; baseline-only>}"

	echo "=== Run current ==="
	(cd "$src" && GITHUB_SHA="$cur" GITHUB_REF="$ref" "$report" run "$out_cur" "${pkgs[@]}") || true
	(cd "$src" && GITHUB_SHA="$cur" GITHUB_REF="$ref" "$report" summary "$out_cur/coverage.out" "$out_cur/coverage-summary.json")
	GITHUB_SHA="$cur" GITHUB_REF="$ref" "$report" timing "$out_cur/test-events.ndjson" "$out_cur/timing-summary.json"

	if [[ -n "$prev" ]]; then
		echo "=== Run previous ==="
		git -C "$src" worktree add --detach "$prevsrc" "$prev"
		(cd "$prevsrc" && GITHUB_SHA="$prev" GITHUB_REF="$ref" "$report" run "$out_prev" "${pkgs[@]}") || true
		if [[ -s "$out_prev/coverage.out" ]]; then
			(cd "$prevsrc" && GITHUB_SHA="$prev" GITHUB_REF="$ref" "$report" summary "$out_prev/coverage.out" "$out_prev/coverage-summary.json") || true
		fi
		if [[ -s "$out_prev/test-events.ndjson" ]]; then
			GITHUB_SHA="$prev" GITHUB_REF="$ref" "$report" timing "$out_prev/test-events.ndjson" "$out_prev/timing-summary.json" || true
		fi
	fi

	local covprev="$out_prev/coverage-summary.json" timprev="$out_prev/timing-summary.json"
	[[ -s "$covprev" ]] || covprev="/nonexistent"
	[[ -s "$timprev" ]] || timprev="/nonexistent"

	echo "=== Compare ==="
	"$report" compare-coverage "$out_cur/coverage-summary.json" "$covprev" "$workdir/comparison-report.json"
	"$report" compare-timing "$out_cur/timing-summary.json" "$timprev" "$workdir/timing-comparison.json"

	echo "=== Render ==="
	GITHUB_STEP_SUMMARY="$workdir/summary.md" "$report" render "$workdir/comparison-report.json" "$workdir/timing-comparison.json"

	echo ""
	echo "########## REPORT ##########"
	cat "$workdir/summary.md"
	echo "############################"
	echo ""
	echo "Artifacts under $workdir:"
	echo "  summary.md, comparison-report.json (+ .html), timing-comparison.json"
	echo "  current/coverage-summary.html"
}

# Run the suite then roll up coverage and timing, tolerating test failures
cmd_collect() {
	local out="${1:?Usage: coverage-report.sh collect <out-dir> [packages...]}"
	shift

	cmd_run "$out" "$@" || echo "Tests reported failures; continuing with collected data." >&2

	if [[ -s "$out/coverage.out" ]]; then
		cmd_summary "$out/coverage.out" "$out/coverage-summary.json"
	else
		echo "No coverage profile at $out/coverage.out; skipping summary." >&2
	fi

	if [[ -s "$out/test-events.ndjson" ]]; then
		cmd_timing "$out/test-events.ndjson" "$out/timing-summary.json"
	else
		echo "No test events at $out/test-events.ndjson; skipping timing." >&2
	fi
}

# Route the subcommand to its handler
main() {
	local cmd="${1:-}"
	shift || true
	case "$cmd" in
	run) cmd_run "$@" ;;
	annotate) cmd_annotate "$@" ;;
	collect) cmd_collect "$@" ;;
	summary) cmd_summary "$@" ;;
	timing) cmd_timing "$@" ;;
	compare-coverage) cmd_compare_coverage "$@" ;;
	compare-timing) cmd_compare_timing "$@" ;;
	render) cmd_render "$@" ;;
	notify) cmd_notify "$@" ;;
	local) cmd_local "$@" ;;
	"" | -h | --help | help) usage ;;
	*)
		echo "Unknown subcommand: $cmd" >&2
		usage >&2
		exit 1
		;;
	esac
}

main "$@"
