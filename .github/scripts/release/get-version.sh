#!/bin/bash

set -e

# Script to get the version from either workflow dispatch input or git ref
# Usage: get-version.sh
# Environment variables:
#   INPUT_TAG: Tag provided via workflow_dispatch
#   EVENT_NAME: GitHub event name (workflow_dispatch or push)
#   GITHUB_REF: Git reference (e.g., refs/tags/v0.93.4)
#   GITHUB_OUTPUT: Path to GitHub output file

function main {
  local version=""

  if [ "$EVENT_NAME" = "workflow_dispatch" ]; then
    version="$INPUT_TAG"
  else
    # Strip refs/tags/ prefix from GITHUB_REF
    version="${GITHUB_REF#refs/tags/}"
  fi

  # Write to GitHub output
  printf 'version=%s\n' "$version" >> "$GITHUB_OUTPUT"
  printf 'Release version: %s\n' "$version"
}

main "$@"
