#!/usr/bin/env bash

set -euo pipefail

# Runs the unit test suite through the sandbox in sandbox-exec.sh.
#
# The vendored OpenTofu packages sit this one out.
#
# Everything Terragrunt maintains itself still runs,
# the hand-written code in `patch` included.
#
# Usage: sandbox-test.sh

CURDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly CURDIR
readonly EXCLUDED='/internal/vendored/opentofu/upstream'
readonly TEST_TIMEOUT="${TEST_TIMEOUT:-45m}"

function main {
	local pkgs
	mapfile -t pkgs < <(go list ./... | grep -v "$EXCLUDED")

	if [[ ${#pkgs[@]} -eq 0 ]]; then
		echo "sandbox-test.sh: go list selected no packages" >&2

		return 1
	fi

	printf 'Testing %d packages, leaving out the generated ones under %s\n' \
		"${#pkgs[@]}" "$EXCLUDED"

	go test \
		-exec "$CURDIR/sandbox-exec.sh" \
		-timeout "$TEST_TIMEOUT" \
		"${pkgs[@]}"
}

main "$@"
