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

    # Validate workflow_dispatch input
    if [[ -z "$version" ]]; then
      echo "ERROR: INPUT_TAG is empty for workflow_dispatch event" >&2
      exit 1
    fi
  else
    # Validate GITHUB_REF exists
    if [[ -z "$GITHUB_REF" ]]; then
      echo "ERROR: GITHUB_REF is empty" >&2
      exit 1
    fi

    # Check if GITHUB_REF is a tag reference
    if [[ ! "$GITHUB_REF" =~ ^refs/tags/ ]]; then
      echo "ERROR: GITHUB_REF does not start with 'refs/tags/': $GITHUB_REF" >&2
      exit 1
    fi

    # Strip refs/tags/ prefix from GITHUB_REF
    version="${GITHUB_REF#refs/tags/}"
  fi

  # Validate extracted version is non-empty
  if [[ -z "$version" ]]; then
    echo "ERROR: Extracted version is empty" >&2
    exit 1
  fi

  # Validate version matches expected pattern (tag-like: starts with letter/digit)
  if [[ ! "$version" =~ ^[a-zA-Z0-9] ]]; then
    echo "ERROR: Invalid version format: '$version' (must start with alphanumeric character)" >&2
    exit 1
  fi

  # Write to GitHub output
  printf 'version=%s\n' "$version" >> "$GITHUB_OUTPUT"
  printf 'Release version: %s\n' "$version"
}

main "$@"
