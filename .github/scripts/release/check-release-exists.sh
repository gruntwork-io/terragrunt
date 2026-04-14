#!/usr/bin/env bash

set -euo pipefail

# Script to check if a GitHub release exists for a given tag
# Usage: check-release-exists.sh
# Environment variables:
#   VERSION: The version/tag to check for
#   GH_TOKEN: GitHub token for authentication
#   GITHUB_OUTPUT: Path to GitHub output file

function main {
  # Validate required environment variables
  : "${VERSION:?ERROR: VERSION is a required environment variable}"
  : "${GH_TOKEN:?ERROR: GH_TOKEN is a required environment variable}"
  : "${GITHUB_OUTPUT:?ERROR: GITHUB_OUTPUT is a required environment variable}"

  printf 'Checking if release exists for tag: %s\n' "$VERSION"

  # Check if release exists using gh CLI (only care about exit code)
  if ! gh release view "$VERSION" > /dev/null 2>&1; then
    printf 'exists=false\n' >> "$GITHUB_OUTPUT"
    printf 'is_draft=false\n' >> "$GITHUB_OUTPUT"
    printf 'is_published=false\n' >> "$GITHUB_OUTPUT"
    printf 'release_id=\n' >> "$GITHUB_OUTPUT"
    printf 'Release not found for tag %s\n' "$VERSION"
    return 0
  fi

  # Get release details
  local release_json
  release_json=$(gh release view "$VERSION" --json 'id,uploadUrl,isDraft')

  local release_id
  local upload_url
  local is_draft
  local is_published

  release_id=$(jq -r '.id' <<< "$release_json")
  upload_url=$(jq -r '.uploadUrl' <<< "$release_json")
  is_draft=$(jq -r '.isDraft' <<< "$release_json")

  if [[ "$is_draft" == "true" ]]; then
    is_published="false"
  else
    is_published="true"
  fi

  # Write to GitHub output
  printf 'exists=true\n' >> "$GITHUB_OUTPUT"
  printf 'release_id=%s\n' "$release_id" >> "$GITHUB_OUTPUT"
  printf 'upload_url=%s\n' "$upload_url" >> "$GITHUB_OUTPUT"
  printf 'is_draft=%s\n' "$is_draft" >> "$GITHUB_OUTPUT"
  printf 'is_published=%s\n' "$is_published" >> "$GITHUB_OUTPUT"

  echo "Found existing release:"
  printf '  Release ID: %s\n' "$release_id"
  printf '  Draft: %s\n' "$is_draft"
  printf '  Published: %s\n' "$is_published"
  printf '  Upload URL: %s\n' "${upload_url%\{*}"

  return 0
}

main "$@"
