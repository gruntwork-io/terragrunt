#!/usr/bin/env bash

set -euo pipefail

JUNIT_FILE="${1:-result.xml}"
OUTPUT="${2:-duration-summary.json}"

if [[ ! -f "$JUNIT_FILE" ]]; then
	echo "Error: JUnit XML file '$JUNIT_FILE' not found" >&2
	exit 1
fi

# Parse <testsuite> elements from go-junit-report JUnit XML.
# Machine-generated format: each testsuite on a single line with name/time/tests/failures attrs.
# Output TSV: name\ttime\ttests\tfailures — jq handles JSON escaping safely.
# Uses POSIX awk (no GNU extensions) for portability across mawk/gawk.
extract_attr() { echo "$1" | sed -n "s/.*$2=\"\([^\"]*\)\".*/\1/p"; }

PACKAGES_JSON=$(grep '<testsuite ' "$JUNIT_FILE" | while IFS= read -r line; do
	name=$(extract_attr "$line" "name")
	time_s=$(extract_attr "$line" "time")
	tests=$(extract_attr "$line" "tests")
	failures=$(extract_attr "$line" "failures")
	[[ -n "$name" ]] && printf '%s\t%s\t%s\t%s\n' "$name" "${time_s:-0}" "${tests:-0}" "${failures:-0}"
done | jq -Rs 'split("\n") | map(select(length > 0) | split("\t") | {(.[0]): (.[1] | tonumber)}) | add // {}')

TOTAL_TIME=$(echo "$PACKAGES_JSON" | jq '[.[] | values] | add // 0')
TOTAL_TESTS=$(grep '<testsuite ' "$JUNIT_FILE" | while IFS= read -r line; do
	extract_attr "$line" "tests"
done | awk '{ sum += $1 } END { print sum+0 }')

jq -n \
	--argjson total_time "$TOTAL_TIME" \
	--argjson total_tests "$TOTAL_TESTS" \
	--arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
	--arg commit "${GITHUB_SHA:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}" \
	--arg ref "${GITHUB_REF:-$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)}" \
	--argjson pkgs "$PACKAGES_JSON" \
	'{total_time: $total_time, total_tests: $total_tests, timestamp: $ts, commit: $commit, ref: $ref, packages: $pkgs}' >"$OUTPUT"

echo "=== Duration: ${TOTAL_TIME}s across ${TOTAL_TESTS} tests ==="
echo ""
jq -r '.packages | to_entries | sort_by(-.value) | .[] | "\(.value)s\t\(.key)"' "$OUTPUT" | column -t -s $'\t'
echo ""
echo "Written to $OUTPUT"
