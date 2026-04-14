#!/usr/bin/env bash

set -euo pipefail

# Script to check if a release edit event contains meaningful changes
# (i.e., changes beyond just the release body/notes).
# Usage: check-edit-meaningful.sh
# Environment variables:
#   CHANGES_JSON: JSON object of changed fields from the GitHub release event
#   GITHUB_OUTPUT: Path to GitHub output file

function main {
  : "${CHANGES_JSON:?ERROR: CHANGES_JSON is a required environment variable}"
  : "${GITHUB_OUTPUT:?ERROR: GITHUB_OUTPUT is a required environment variable}"

  local non_body_keys
  non_body_keys=$(jq -r 'keys[]' <<<"$CHANGES_JSON" | grep -v '^body$' || true)

  if [[ -z "$non_body_keys" ]]; then
    printf 'skip=true\n' >>"$GITHUB_OUTPUT"
    echo "Only release notes changed, skipping rebuild"
  else
    printf 'skip=false\n' >>"$GITHUB_OUTPUT"
    echo "Meaningful changes detected: $non_body_keys"
  fi

  return 0
}

main "$@"
