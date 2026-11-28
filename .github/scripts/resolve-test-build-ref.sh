#!/usr/bin/env bash

set -euo pipefail

# Script to resolve the commit and S3 prefix for a test build
# Usage: resolve-test-build-ref.sh
#
# Environment variables:
#   PR_NUMBER         - Pull request to build; empty builds GITHUB_SHA (optional)
#   GH_TOKEN          - Token for the GitHub API (required with PR_NUMBER)
#   GITHUB_REPOSITORY - Repository in owner/name form (required)
#   GITHUB_SHA        - Commit of the triggering ref (required)
#   GITHUB_REF        - Triggering ref (required)
#   GITHUB_OUTPUT     - File that receives the sha and prefix outputs (required)

function main {
	local sha="$GITHUB_SHA"
	local prefix="test"

	if [[ -z "${PR_NUMBER:-}" && "$GITHUB_REF" == "refs/heads/main" ]]; then
		prefix="tip"
	fi

	if [[ -n "${PR_NUMBER:-}" ]]; then
		if [[ ! "$PR_NUMBER" =~ ^[1-9][0-9]{0,9}$ ]]; then
			echo "ERROR: PR_NUMBER must be a positive integer, got '${PR_NUMBER}'" >&2
			exit 1
		fi

		sha="$(gh api "repos/${GITHUB_REPOSITORY}/pulls/${PR_NUMBER}" --jq '.head.sha')"

		if [[ ! "$sha" =~ ^[0-9a-f]{40}$ ]]; then
			echo "ERROR: Could not resolve the head commit of pull request #${PR_NUMBER}" >&2
			exit 1
		fi
	fi

	echo "Building ${prefix}-${sha}"

	{
		echo "sha=${sha}"
		echo "prefix=${prefix}"
	} >>"$GITHUB_OUTPUT"
}

main "$@"
