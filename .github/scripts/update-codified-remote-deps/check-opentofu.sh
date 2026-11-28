#!/usr/bin/env bash

# Check for an OpenTofu release newer than the one mise.toml pins.
#
# Usage: check-opentofu.sh

set -euo pipefail

# shellcheck source=lib.sh
source "$(dirname "$0")/lib.sh"

readonly CONFIG_FILE="mise.toml"
readonly MINIMUM_RELEASE_AGE="7d"
readonly VERSION_PATTERN='^[0-9]+\.[0-9]+\.[0-9]+$'

main() {
	local current latest

	require_tools mise grep tr
	require_env GITHUB_OUTPUT

	current="$(find_pin '(?m)^opentofu = "\K[^"]+' "$CONFIG_FILE")"
	latest="$(mise latest --minimum-release-age "$MINIMUM_RELEASE_AGE" opentofu)"
	readonly current latest

	write_outputs "$current" "$latest" "$VERSION_PATTERN"
}

main "$@"
