#!/usr/bin/env bash

set -euo pipefail

# Script to resolve the release version and target ref from a
# workflow_dispatch input by looking up the existing draft release.
#
# Usage: resolve-version-ref.sh
# Environment variables:
#   INPUT_VERSION: Version provided via workflow_dispatch (required)
#   GH_TOKEN: GitHub token for authentication (required)
#   GITHUB_OUTPUT: Path to GitHub output file

function main {
	: "${INPUT_VERSION:?ERROR: INPUT_VERSION is a required environment variable}"
	: "${GH_TOKEN:?ERROR: GH_TOKEN is a required environment variable}"
	: "${GITHUB_OUTPUT:?ERROR: GITHUB_OUTPUT is a required environment variable}"

	local version="$INPUT_VERSION"

	# Look up the release to get target_commitish
	local release_json
	if ! release_json=$(gh release view "$version" --json 'isDraft,targetCommitish' 2>/dev/null); then
		echo "ERROR: No release found for version $version" >&2
		exit 1
	fi

	local is_draft
	is_draft=$(jq -r '.isDraft' <<<"$release_json")
	if [[ "$is_draft" != "true" ]]; then
		echo "ERROR: Release $version is already published. Cannot rebuild a published release." >&2
		exit 1
	fi

	local ref
	ref=$(jq -r '.targetCommitish' <<<"$release_json")

	{
		printf 'version=%s\n' "$version"
		printf 'ref=%s\n' "$ref"
	} >>"$GITHUB_OUTPUT"
	echo "Resolved version: $version, ref: $ref"

	return 0
}

main "$@"
