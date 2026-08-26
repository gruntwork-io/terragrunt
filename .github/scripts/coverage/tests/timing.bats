#!/usr/bin/env bats

setup() {
	SCRIPT="${BATS_TEST_DIRNAME}/../coverage-report.sh"
	CURRENT="${BATS_TEST_TMPDIR}/current.json"
	PREVIOUS="${BATS_TEST_TMPDIR}/previous.json"
	REPORT="${BATS_TEST_TMPDIR}/report.json"
	SUPPRESSIONS="${BATS_TEST_TMPDIR}/report-suppressions.json"

	cat >"$CURRENT" <<'EOF'
{
  "total_sec": 50,
  "packages": {
    "example.com/a": {
      "wall_sec": 10,
      "tests": {
        "TestCAS_CloneRepoWithNestedSubmodules": 9,
        "TestCAS_CloneRepoWithNestedSubmodules/child": 8,
        "TestCAS_CloneRepoWithNestedSubmodulesSlow": 7,
        "TestOther": 6
      }
    },
    "example.com/ignored": {
      "wall_sec": 20,
      "tests": {
        "TestIgnored": 19
      }
    },
    "example.com/ignored/child": {
      "wall_sec": 18,
      "tests": {
        "TestIgnoredChild": 17
      }
    }
  }
}
EOF

	cat >"$PREVIOUS" <<'EOF'
{
  "total_sec": 35,
  "packages": {
    "example.com/a": {
      "wall_sec": 8,
      "tests": {
        "TestCAS_CloneRepoWithNestedSubmodules": 7,
        "TestCAS_CloneRepoWithNestedSubmodules/child": 6,
        "TestCAS_CloneRepoWithNestedSubmodulesSlow": 5,
        "TestOther": 4
      }
    },
    "example.com/ignored": {
      "wall_sec": 15,
      "tests": {
        "TestIgnored": 14
      }
    },
    "example.com/ignored/child": {
      "wall_sec": 12,
      "tests": {
        "TestIgnoredChild": 11
      }
    }
  }
}
EOF
}

@test "suppresses configured tests and packages from timing diffs" {
	cat >"$SUPPRESSIONS" <<'EOF'
{
  "tests": ["TestCAS_CloneRepoWithNestedSubmodules"],
  "packages": ["example.com/ignored"]
}
EOF

	REPORT_SUPPRESSIONS_FILE="$SUPPRESSIONS" run "$SCRIPT" compare-timing "$CURRENT" "$PREVIOUS" "$REPORT"
	[ "$status" -eq 0 ]

	run jq -e '.slow_packages | map(.package) == ["example.com/a"]' "$REPORT"
	[ "$status" -eq 0 ]

	run jq -e '.slow_packages[0].top_tests | map(.name) == [
    "TestCAS_CloneRepoWithNestedSubmodulesSlow",
    "TestOther"
  ]' "$REPORT"
	[ "$status" -eq 0 ]

	run jq -e '.top_regressions | map(.package) == ["example.com/a"]' "$REPORT"
	[ "$status" -eq 0 ]

	run jq -r '[.current_total_sec, .previous_total_sec, .slow_packages[0].current_sec] | @tsv' "$REPORT"
	[ "$output" = $'50\t35\t10' ]
}

@test "uses no report suppressions when the config is missing" {
	REPORT_SUPPRESSIONS_FILE="${BATS_TEST_TMPDIR}/missing.json" run "$SCRIPT" compare-timing "$CURRENT" "$PREVIOUS" "$REPORT"
	[ "$status" -eq 0 ]

	run jq -r '.slow_packages[].package' "$REPORT"
	[[ "$output" == *"example.com/ignored"* ]]

	run jq -r '.slow_packages[].top_tests[].name' "$REPORT"
	[[ "$output" == *"TestCAS_CloneRepoWithNestedSubmodules"* ]]
}

@test "uses the repository report suppression config by default" {
	run "$SCRIPT" compare-timing "$CURRENT" "$PREVIOUS" "$REPORT"
	[ "$status" -eq 0 ]

	run jq -e '[.slow_packages[].top_tests[].name] | index("TestCAS_CloneRepoWithNestedSubmodules") == null' "$REPORT"
	[ "$status" -eq 0 ]

	run jq -e '.slow_packages | map(.package) | index("example.com/ignored") != null' "$REPORT"
	[ "$status" -eq 0 ]
}

@test "suppresses configured tests and packages from baseline reports" {
	cat >"$SUPPRESSIONS" <<'EOF'
{
  "tests": ["TestCAS_CloneRepoWithNestedSubmodules"],
  "packages": ["example.com/ignored"]
}
EOF

	REPORT_SUPPRESSIONS_FILE="$SUPPRESSIONS" run "$SCRIPT" compare-timing "$CURRENT" "${BATS_TEST_TMPDIR}/missing-previous.json" "$REPORT"
	[ "$status" -eq 0 ]

	run jq -e '.slow_packages | map(.package) == ["example.com/a"]' "$REPORT"
	[ "$status" -eq 0 ]

	run jq -e '.slow_packages[0].top_tests | map(.name) == [
    "TestCAS_CloneRepoWithNestedSubmodulesSlow",
    "TestOther"
  ]' "$REPORT"
	[ "$status" -eq 0 ]
}
