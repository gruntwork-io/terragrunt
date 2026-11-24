#!/usr/bin/env bash

# Check for a Terraform release newer than the one terraform-switch-latest.sh
# installs.
#
# Usage: check-terraform.sh

set -euo pipefail

# shellcheck source=lib.sh
source "$(dirname "$0")/lib.sh"

readonly SWITCH_SCRIPT=".github/scripts/setup/terraform-switch-latest.sh"
readonly MINIMUM_RELEASE_AGE="7d"
readonly VERSION_PATTERN='^[0-9]+\.[0-9]+\.[0-9]+$'

main() {
	local current latest

	require_tools mise grep tr
	require_env GITHUB_OUTPUT

	current="$(find_pin 'terraform-switch\.sh \K[0-9.]+' "$SWITCH_SCRIPT")"
	latest="$(mise latest --minimum-release-age "$MINIMUM_RELEASE_AGE" terraform)"
	readonly current latest

	write_outputs "$current" "$latest" "$VERSION_PATTERN"
}

main "$@"
