#!/usr/bin/env bash

set -euo pipefail

# Script to validate a version string is strict semantic versioning with a 'v' prefix.
# Usage: validate-semver.sh <version>
# Or via environment variable:
#   VERSION=v1.0.0 validate-semver.sh
#
# Accepted formats:
#   v1.0.0, v1.0.0-rc1, v1.0.0-alpha.1, v1.0.0-beta.2
# Rejected formats:
#   alpha2025031301, 1.0.0 (no prefix), v1 (incomplete)

# Semver 2.0.0 regex with required 'v' prefix:
#   v<major>.<minor>.<patch>[-<pre-release>][+<build>]
# Pre-release and build identifiers are dot-separated, [0-9A-Za-z-].
# Note: this does not enforce the SemVer rule that numeric pre-release
# identifiers must not have leading zeros.
SEMVER_RE='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'

function validate_semver {
	local -r version="$1"

	if [[ -z "$version" ]]; then
		echo "ERROR: Version string is empty" >&2
		return 1
	fi

	if [[ ! "$version" =~ $SEMVER_RE ]]; then
		echo "ERROR: '$version' is not valid semver. Expected format: v<major>.<minor>.<patch>[-<pre-release>]" >&2
		echo "Examples: v1.0.0, v1.0.0-rc1, v1.0.0-alpha.1" >&2
		return 1
	fi

	return 0
}

function main {
	local version="${1:-${VERSION:-}}"

	if [[ -z "$version" ]]; then
		echo "ERROR: No version provided. Pass as argument or set VERSION env var." >&2
		exit 1
	fi

	validate_semver "$version"

	printf 'Valid semver: %s\n' "$version"

	return 0
}

main "$@"
