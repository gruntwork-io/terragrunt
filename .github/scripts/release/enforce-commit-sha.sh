#!/usr/bin/env bash

set -euo pipefail

# Script to enforce that a git ref is a full 40-character commit SHA.
# This prevents branch names from being used as release targets, which
# could allow new commits to be pulled into the release between draft
# creation and publishing.
#
# Usage: enforce-commit-sha.sh <ref>
# Or via environment variable:
#   REF=abc123... enforce-commit-sha.sh

function main {
  local ref="${1:-${REF:-}}"

  if [[ -z "$ref" ]]; then
    echo "ERROR: No ref provided. Pass as argument or set REF env var." >&2
    exit 1
  fi

  if [[ ! "$ref" =~ ^[0-9a-f]{40}$ ]]; then
    echo "ERROR: target_commitish must be a full 40-character commit SHA, got: '$ref'" >&2
    echo "This prevents new commits from being pulled into the release between draft creation and publishing." >&2
    echo "When creating the draft release, select a specific commit instead of a branch name." >&2
    exit 1
  fi

  printf 'Valid commit SHA: %s\n' "$ref"

  return 0
}

main "$@"
