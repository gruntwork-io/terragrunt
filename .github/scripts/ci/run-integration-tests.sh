#!/usr/bin/env bash

set -euo pipefail

# Run one entry of the integration matrix and record a JUnit report and the
# go test -json event stream for it.
#
# --rerun-fails retries only the failed tests, up to twice, so a transient cloud
# API or registry error does not fail the whole leg. gotestsum skips the reruns
# when the first pass has more than 10 failures, so a real breakage still fails
# fast. Every attempt lands in result.xml, where the report step lists tests that
# passed on a rerun as flaky. gotestsum replaces the -run filter with the failed
# test's name on a rerun, and requires the packages in --packages.
#
# --max-fails ends the leg once 25 tests have failed. A failure that wide means
# broken credentials or a provider outage, and the rest of the run would only
# report more of the same.

: "${GITHUB_WORKSPACE:?GITHUB_WORKSPACE is not set}"
: "${TARGET:?TARGET is not set}"

TAGS="${TAGS:-}"
RUN="${RUN:-}"
TEST_ARGS="${TEST_ARGS:-}"
HAS_DOCKER="${HAS_DOCKER:-false}"

# shellcheck source=/dev/null
source "${GITHUB_WORKSPACE}/.env.secrets"

if [[ "$HAS_DOCKER" == "true" ]]; then
	TAGS="${TAGS:+${TAGS},}docker"
fi

args=(-timeout 45m)

if [[ -n "$TAGS" ]]; then
	args+=(-tags "$TAGS")
fi

if [[ -n "$RUN" ]]; then
	args+=(-run "$RUN")
fi

if [[ -n "$TEST_ARGS" ]]; then
	read -r -a split_test_args <<<"$TEST_ARGS"
	args+=("${split_test_args[@]}")
fi

set -x
gotestsum \
	--format github-actions \
	--hide-summary skipped \
	--junitfile result.xml \
	--junitfile-hide-empty-pkg \
	--junitfile-testcase-classname relative \
	--junitfile-testsuite-name relative \
	--jsonfile test-events.ndjson \
	--rerun-fails=2 \
	--rerun-fails-abort-on-data-race \
	--rerun-fails-report rerun-report.txt \
	--max-fails 25 \
	--packages "$TARGET" \
	-- "${args[@]}"
