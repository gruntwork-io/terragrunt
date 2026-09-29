#!/usr/bin/env bats

setup() {
	# Keep any summary a script writes inside this test's tmpdir, not in the job summary.
	export GITHUB_STEP_SUMMARY="${BATS_TEST_TMPDIR}/summary.md"

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

@test "timing omits packages that ran no tests" {
	EVENTS="${BATS_TEST_TMPDIR}/events.ndjson"

	# Under -cover, a package with no test files still gets a package-level pass
	# whose Elapsed is build time (6.6s here for a main package), and without
	# -cover it gets a [no test files] skip. Neither is test runtime.
	cat >"$EVENTS" <<'EOF2'
{"Action":"start","Package":"example.com/root"}
{"Action":"output","Package":"example.com/root","Output":"\texample.com/root\t\tcoverage: 0.0% of statements\n"}
{"Action":"pass","Package":"example.com/root","Elapsed":6.6}
{"Action":"skip","Package":"example.com/notests","Elapsed":0.2}
{"Action":"run","Package":"example.com/a","Test":"TestA"}
{"Action":"pass","Package":"example.com/a","Test":"TestA","Elapsed":1.5}
{"Action":"pass","Package":"example.com/a","Elapsed":2}
{"Action":"run","Package":"example.com/skipped","Test":"TestSkipped"}
{"Action":"skip","Package":"example.com/skipped","Test":"TestSkipped","Elapsed":0}
{"Action":"pass","Package":"example.com/skipped","Elapsed":0.1}
EOF2

	run "$SCRIPT" timing "$EVENTS" "$REPORT"
	[ "$status" -eq 0 ]

	run jq -e '.packages | keys == ["example.com/a", "example.com/skipped"]' "$REPORT"
	[ "$status" -eq 0 ]

	run jq -e '.total_sec == 2.1' "$REPORT"
	[ "$status" -eq 0 ]
}
