#!/usr/bin/env bash

set -euo pipefail

# Run one entry of the integration matrix and record a JUnit report for it.

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

args=(-v -timeout 45m)

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
go test "${args[@]}" "$TARGET" | tee test_output.log
go-junit-report <test_output.log >result.xml
