#!/usr/bin/env bash

set -euo pipefail

# Script to create a draft GitHub release for a version that has none.
# A version with a pre-release suffix (e.g., v1.0.0-rc1) is marked as a
# pre-release.
#
# Usage: create-draft-release.sh
# Environment variables:
#   VERSION: The version/tag to create the release for (required)
#   TARGET: Full commit SHA the release tag will point at (required)
#   NOTES_FILE: Path to a file holding the release notes (required)
#   GH_TOKEN: GitHub token for authentication (required)

function main {
	: "${VERSION:?ERROR: VERSION is a required environment variable}"
	: "${TARGET:?ERROR: TARGET is a required environment variable}"
	: "${NOTES_FILE:?ERROR: NOTES_FILE is a required environment variable}"
	: "${GH_TOKEN:?ERROR: GH_TOKEN is a required environment variable}"

	if [[ ! -s "$NOTES_FILE" ]]; then
		echo "ERROR: Release notes file '$NOTES_FILE' is missing or empty" >&2
		exit 1
	fi

	local -a args=(
		"$VERSION"
		--draft
		--title "$VERSION"
		--target "$TARGET"
		--notes-file "$NOTES_FILE"
	)

	local kind="release"
	if [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+- ]]; then
		args+=(--prerelease)
		kind="pre-release"
	fi

	gh release create "${args[@]}"
	printf 'Created draft %s %s targeting %s\n' "$kind" "$VERSION" "$TARGET"

	return 0
}

main "$@"
