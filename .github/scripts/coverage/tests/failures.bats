#!/usr/bin/env bats

# Required for `run --separate-stderr`, which pins where the annotation is written.
bats_require_minimum_version 1.5.0

setup() {
  SCRIPT="${BATS_TEST_DIRNAME}/../coverage-report.sh"
  EVENTS="${BATS_TEST_TMPDIR}/events.ndjson"

  cat >"$EVENTS" <<'EOF'
{"Action":"run","Package":"example.com/a","Test":"TestFails"}
{"Action":"output","Package":"example.com/a","Test":"TestFails","Output":"    a_test.go:9: boom: expected 1 got 2\n"}
{"Action":"fail","Package":"example.com/a","Test":"TestFails","Elapsed":0.01}
{"Action":"output","Package":"example.com/a","Test":"TestPasses","Output":"--- PASS: TestPasses\n"}
{"Action":"pass","Package":"example.com/a","Test":"TestPasses","Elapsed":0.01}
{"Action":"fail","Package":"example.com/a","Elapsed":0.02}
{"ImportPath":"example.com/b [example.com/b.test]","Action":"build-output","Output":"b/b_test.go:6:2: undefined: undefinedSymbol\n"}
{"ImportPath":"example.com/b [example.com/b.test]","Action":"build-fail"}
EOF
}

@test "names the failing test" {
  run "$SCRIPT" failures "$EVENTS"
  [ "$status" -eq 0 ]
  [[ "$output" == *"TestFails"* ]]
  [[ "$output" == *"example.com/a"* ]]
}

@test "replays the failing test output" {
  run "$SCRIPT" failures "$EVENTS"
  [[ "$output" == *"boom: expected 1 got 2"* ]]
}

@test "does not report passing tests" {
  run "$SCRIPT" failures "$EVENTS"
  [[ "$output" != *"TestPasses"* ]]
}

@test "surfaces a build error, keyed by ImportPath rather than Package" {
  run "$SCRIPT" failures "$EVENTS"
  [[ "$output" == *"undefined: undefinedSymbol"* ]]
  [[ "$output" == *"example.com/b"* ]]
}

@test "emits an error annotation naming the failures, on stderr" {
  run --separate-stderr "$SCRIPT" failures "$EVENTS"
  [[ "$stderr" == *"::error title=Failing tests (1)::TestFails"* ]]
  [[ "$output" != *"::error"* ]]
}

@test "writes the failing tests to the step summary" {
  GITHUB_STEP_SUMMARY="${BATS_TEST_TMPDIR}/summary.md" run "$SCRIPT" failures "$EVENTS"
  run cat "${BATS_TEST_TMPDIR}/summary.md"
  [[ "$output" == *"TestFails"* ]]
}

@test "reports what a cap dropped" {
  FAILURE_OUTPUT_LINES=0 run "$SCRIPT" failures "$EVENTS"
  [[ "$output" == *"omitted"* ]]
}

@test "falls back to the stream tail when no failure event was recorded" {
  local partial="${BATS_TEST_TMPDIR}/partial.ndjson"
  head -n 2 "$EVENTS" >"$partial"

  run "$SCRIPT" failures "$partial"
  [ "$status" -eq 0 ]
  [[ "$output" == *"printing the tail of the stream"* ]]
  [[ "$output" == *"boom: expected 1 got 2"* ]]
}

@test "tolerates a missing event stream" {
  run "$SCRIPT" failures "${BATS_TEST_TMPDIR}/nonexistent.ndjson"
  [ "$status" -eq 0 ]
  [[ "$output" == *"cannot report failing tests"* ]]
}
