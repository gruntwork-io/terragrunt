#!/usr/bin/env bash

# Check terragrunt-infrastructure-live-stacks-example for a commit newer than
# the one TestExampleLiveStacks checks out.
#
# Usage: check-live-stacks-example.sh

set -euo pipefail

# shellcheck source=lib.sh
source "$(dirname "$0")/lib.sh"

readonly REPO="https://github.com/gruntwork-io/terragrunt-infrastructure-live-stacks-example.git"
readonly TEST_FILE="test/integration_example_live_stacks_test.go"
readonly COMMIT_PATTERN='^[0-9a-f]{40}$'

main() {
	local current latest

	require_tools git awk grep tr
	require_env GITHUB_OUTPUT

	current="$(find_pin '"checkout",\s*"\K[0-9a-f]{40}' "$TEST_FILE")"
	latest="$(git ls-remote "$REPO" HEAD | awk '{print $1}')"
	readonly current latest

	write_outputs "$current" "$latest" "$COMMIT_PATTERN"
}

main "$@"
