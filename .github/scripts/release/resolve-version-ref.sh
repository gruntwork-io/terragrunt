#!/usr/bin/env bash

set -euo pipefail

# Script to resolve the release version and target ref from either a
# release event or a workflow_dispatch input.
#
# For workflow_dispatch: looks up the existing draft release by version
# and extracts the target commitish.
# For release events: reads version and ref directly from event context.
#
# Usage: resolve-version-ref.sh
# Environment variables:
#   EVENT_NAME: GitHub event name (workflow_dispatch or release)
#   INPUT_VERSION: Version provided via workflow_dispatch (required for workflow_dispatch)
#   RELEASE_TAG_NAME: Tag name from the release event (required for release events)
#   RELEASE_TARGET: Target commitish from the release event (required for release events)
#   GH_TOKEN: GitHub token for authentication (required for workflow_dispatch)
#   GITHUB_OUTPUT: Path to GitHub output file

function main {
  : "${EVENT_NAME:?ERROR: EVENT_NAME is a required environment variable}"
  : "${GITHUB_OUTPUT:?ERROR: GITHUB_OUTPUT is a required environment variable}"

  local version
  local ref

  if [[ "$EVENT_NAME" == "workflow_dispatch" ]]; then
    : "${INPUT_VERSION:?ERROR: INPUT_VERSION is required for workflow_dispatch}"
    : "${GH_TOKEN:?ERROR: GH_TOKEN is required for workflow_dispatch}"

    version="$INPUT_VERSION"

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

    ref=$(jq -r '.targetCommitish' <<<"$release_json")
  else
    : "${RELEASE_TAG_NAME:?ERROR: RELEASE_TAG_NAME is required for release events}"
    : "${RELEASE_TARGET:?ERROR: RELEASE_TARGET is required for release events}"

    version="$RELEASE_TAG_NAME"
    ref="$RELEASE_TARGET"
  fi

  printf 'version=%s\n' "$version" >>"$GITHUB_OUTPUT"
  printf 'ref=%s\n' "$ref" >>"$GITHUB_OUTPUT"
  echo "Resolved version: $version, ref: $ref"

  return 0
}

main "$@"
