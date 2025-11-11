#!/bin/bash

set -e

# Script to get the version from either workflow dispatch input or git ref
# Usage: get-version.sh
# Environment variables:
#   INPUT_TAG: Tag provided via workflow_dispatch
#   EVENT_NAME: GitHub event name (workflow_dispatch or push)
#   GITHUB_REF: Git reference (e.g., refs/tags/v0.93.4)
#   GITHUB_OUTPUT: Path to GitHub output file

function resolve_version {
  # Handle workflow_dispatch event (manual trigger with INPUT_TAG)
  if [ "$EVENT_NAME" = "workflow_dispatch" ]; then
    if [[ -z "$INPUT_TAG" ]]; then
      echo "ERROR: INPUT_TAG is empty for workflow_dispatch event" >&2
      exit 1
    fi
    echo "$INPUT_TAG"
    return 0
  fi

  # Handle push event (tag push with GITHUB_REF)
  if [[ -z "$GITHUB_REF" ]]; then
    echo "ERROR: GITHUB_REF is empty" >&2
    exit 1
  fi

  if [[ ! "$GITHUB_REF" =~ ^refs/tags/ ]]; then
    echo "ERROR: GITHUB_REF does not start with 'refs/tags/': $GITHUB_REF" >&2
    exit 1
  fi

  # Strip refs/tags/ prefix and return
  echo "${GITHUB_REF#refs/tags/}"
}

function validate_version {
  local -r version="$1"

  if [[ -z "$version" ]]; then
    echo "ERROR: Extracted version is empty" >&2
    exit 1
  fi

  # Validate version matches expected pattern (tag-like: starts with letter/digit)
  if [[ ! "$version" =~ ^[a-zA-Z0-9] ]]; then
    echo "ERROR: Invalid version format: '$version' (must start with alphanumeric character)" >&2
    exit 1
  fi
}

function main {
  local version
  version=$(resolve_version)

  validate_version "$version"

  # Write to GitHub output
  printf 'version=%s\n' "$version" >> "$GITHUB_OUTPUT"
  printf 'Release version: %s\n' "$version"
}

main "$@"
