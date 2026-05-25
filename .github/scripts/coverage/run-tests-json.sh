#!/usr/bin/env bash

# Runs the unit-test suite and emits three outputs into <out-dir>:
#   coverage.out         Go cover profile
#   test-events.ndjson   raw `go test -json` event stream
#   result.xml           JUnit XML (converted from ndjson via go-junit-report -parser gojson)
#
# Exit code propagates the underlying `go test` status; failure to convert to JUnit
# is treated as fatal so the artifact set stays consistent.

set -euo pipefail

OUT="${1:?Usage: run-tests-json.sh <out-dir> [extra go-test args...]}"
shift || true

mkdir -p "$OUT"
EVENTS="$OUT/test-events.ndjson"
COVER="$OUT/coverage.out"
JUNIT="$OUT/result.xml"

# Run the test suite. Capture every -json event to ndjson while teeing through
# go-junit-report to produce JUnit XML.
set +e
go test -json -coverprofile="$COVER" -covermode=atomic "$@" ./... -timeout 45m |
	tee "$EVENTS" |
	go-junit-report -parser gojson -set-exit-code >"$JUNIT"
STATUS=${PIPESTATUS[0]}
set -e

echo "go test exit status: $STATUS"
echo "Events: $EVENTS ($(wc -l <"$EVENTS") lines)"
echo "Cover:  $COVER"
echo "JUnit:  $JUNIT"

exit "$STATUS"
