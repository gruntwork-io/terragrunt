#!/usr/bin/env bash

set -euo pipefail

# List the tests that failed in a go test log.

: "${NAME:?NAME is not set}"

readonly log_file="${1:-test_output.log}"

echo "Failed Tests in ${NAME}"

if [[ ! -f "$log_file" ]]; then
	echo "No test output found"
	exit 0
fi

# grep exits non-zero when it matches nothing, which pipefail would treat as a
# script failure rather than a run without failures.
failed_count="$(grep -c -E '^--- FAIL:' "$log_file" || true)"

echo "Failed tests count: ${failed_count}"

if [[ "$failed_count" -eq 0 ]]; then
	echo "No failed tests found"
	exit 0
fi

echo "Failed test names:"
grep -E '^--- FAIL:' "$log_file" | sed 's/.*FAIL:[[:space:]]*//'
