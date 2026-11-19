#!/usr/bin/env bats

setup() {
	SCRIPT="${BATS_TEST_DIRNAME}/../notify-slack.sh"
	REPORT="${BATS_TEST_TMPDIR}/report.json"

	export COVERAGE_SLACK_WEBHOOK_URL="https://hooks.example.com/fake-webhook"
	export REPO="example/repo"
	export GITHUB_RUN_ID="42"
	export CURL_ARGS="${BATS_TEST_TMPDIR}/curl-args"

	# A curl that records its arguments, one per line, and sends nothing.
	mkdir -p "${BATS_TEST_TMPDIR}/bin"
	cat >"${BATS_TEST_TMPDIR}/bin/curl" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" >"$CURL_ARGS"
EOF

	chmod +x "${BATS_TEST_TMPDIR}/bin/curl"

	export PATH="${BATS_TEST_TMPDIR}/bin:${PATH}"
}

# Write a report with one metric per verdict given.
write_report() {
	local regressions=0 improvements=0 metrics="" verdict delta

	for verdict in "$@"; do
		case "$verdict" in
		regression)
			regressions=$((regressions + 1))
			delta=12.34
			;;
		improvement)
			improvements=$((improvements + 1))
			delta=-8.5
			;;
		*)
			delta=0.4
			;;
		esac

		metrics+="${metrics:+,}{\"name\":\"time_wall_clock\",\"source\":\"hyperfine\",\"deltaPct\":${delta},\"verdict\":\"${verdict}\"}"
	done

	cat >"$REPORT" <<EOF
{
  "comparison": "previous",
  "baseSha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "headSha": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "regressions": ${regressions},
  "improvements": ${improvements},
  "scenarios": [{"id": "find.flat.100", "metrics": [${metrics}]}]
}
EOF
}

# Print the text of the message the script posted.
posted_text() {
	grep -F '"text"' "$CURL_ARGS" | jq -r '.text'
}

@test "sends nothing when no metric is flagged" {
	write_report unchanged unconfirmed

	run "$SCRIPT" "$REPORT"
	[ "$status" -eq 0 ]
	[[ "$output" == *"Skipping Slack notification"* ]]
	[ ! -e "$CURL_ARGS" ]
}

@test "posts regressions and improvements with links to the commits and the run" {
	write_report regression improvement unchanged

	run "$SCRIPT" "$REPORT"
	[ "$status" -eq 0 ]

	grep -qxF "$COVERAGE_SLACK_WEBHOOK_URL" "$CURL_ARGS"

	text="$(posted_text)"
	[[ "$text" == *"Benchmark (previous)"* ]]
	[[ "$text" == *"1 regressed and 1 improved metrics"* ]]
	[[ "$text" == *'`find.flat.100` hyperfine time_wall_clock: +12.3%'* ]]
	[[ "$text" == *'`find.flat.100` hyperfine time_wall_clock: -8.5%'* ]]
	[[ "$text" == *"https://github.com/example/repo/compare/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa...bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"* ]]
	[[ "$text" == *"https://github.com/example/repo/actions/runs/42"* ]]
}

@test "omits the improvements section when only regressions are flagged" {
	write_report regression

	run "$SCRIPT" "$REPORT"
	[ "$status" -eq 0 ]

	text="$(posted_text)"
	[[ "$text" == *"Regressions:"* ]]
	[[ "$text" != *"Improvements:"* ]]
}

@test "fails when the report is missing" {
	run "$SCRIPT" "${BATS_TEST_TMPDIR}/missing.json"
	[ "$status" -eq 1 ]
	[[ "$output" == *"not found"* ]]
}
