#!/bin/bash

set -e

# Script to upload release assets to GitHub
# Usage: upload-assets.sh <bin-directory>
# Environment variables:
#   VERSION: The version/tag to upload to
#   GH_TOKEN: GitHub token for authentication
#   CLOBBER: Set to 'true' to overwrite existing assets (default: false)

function main {
  local -r bin_dir="${1:-bin}"
  local -r clobber="${CLOBBER:-false}"

  # Validate required environment variables
  : "${VERSION:?ERROR: VERSION is a required environment variable}"
  : "${GH_TOKEN:?ERROR: GH_TOKEN is a required environment variable}"

  if [[ ! -d "$bin_dir" ]]; then
    echo "ERROR: Directory $bin_dir does not exist"
    exit 1
  fi

  # Build upload command with optional --clobber flag
  local clobber_flag=""
  if [[ "$clobber" == "true" ]]; then
    clobber_flag="--clobber"
    echo "Note: --clobber enabled - will overwrite existing assets"
  else
    echo "Note: --clobber disabled - will fail if assets already exist"
  fi

  printf 'Uploading assets to existing release %s...\n' "$VERSION"

  # Use pushd/popd to avoid side effects on caller's working directory
  pushd "$bin_dir" || return 1

  # Upload all files using gh CLI
  for file in *; do
    echo "Uploading $file..."
    if gh release upload "$VERSION" "$file" $clobber_flag; then
      echo "Uploaded $file"
    else
      echo "Upload failed for $file (will retry in verification)"
    fi
  done

  # Return to original directory
  popd || return 1

  echo "Upload phase completed"
}

main "$@"
