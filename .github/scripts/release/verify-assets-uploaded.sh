#!/bin/bash

set -e

# Script to verify all assets were uploaded to the GitHub release
# Usage: verify-assets-uploaded.sh <bin-directory>
# Environment variables:
#   VERSION: The version/tag to verify
#   GH_TOKEN: GitHub token for authentication
#   CLOBBER: Set to 'true' to overwrite existing assets during retry (default: false)

# Source configuration library
# shellcheck source=lib-release-config.sh
source "$(dirname "$0")/lib-release-config.sh"

readonly MAX_RETRIES=10

function main {
  local -r bin_dir="${1:-bin}"
  local -r clobber="${CLOBBER:-false}"

  # Validate required environment variables
  : "${VERSION:?ERROR: VERSION is a required environment variable}"
  : "${GH_TOKEN:?ERROR: GH_TOKEN is a required environment variable}"
  verify_config_file

  # Build upload command with optional --clobber flag
  local clobber_flag=""
  if [[ "$clobber" == "true" ]]; then
    clobber_flag="--clobber"
  fi

  echo "Verifying all assets are accessible..."

  # Get list of assets in the release
  local assets
  assets=$(gh release view "$VERSION" --json 'assets' --jq '.assets[].name')

  local asset_count
  asset_count=$(wc -l <<< "$assets")

  echo "Found $asset_count assets in release"

  # Get expected files from centralized configuration
  local expected_files
  mapfile -t expected_files < <(get_all_expected_files)

  # Check each expected file
  for expected_file in "${expected_files[@]}"; do
    echo "Checking $expected_file..."

    # Check if file exists in release
    if ! grep -q "^${expected_file}$" <<< "$assets"; then
      echo "$expected_file not found in release, uploading..."

      # Upload the missing file
      if [ -f "$bin_dir/$expected_file" ]; then
        local i
        for ((i=0; i<MAX_RETRIES; i++)); do
          if gh release upload "$VERSION" "$bin_dir/$expected_file" $clobber_flag; then
            echo "Uploaded $expected_file"
            break
          fi

          echo "Upload attempt $((i+1))/$MAX_RETRIES failed"
          sleep 5
        done

        if (( i == MAX_RETRIES )); then
          echo "Failed to upload $expected_file after $MAX_RETRIES retries"
          exit 1
        fi
      else
        echo "File $bin_dir/$expected_file not found locally"
        exit 1
      fi
    else
      echo "$expected_file present"
    fi
  done

  # Verify we can download assets (spot check)
  echo ""
  echo "Verifying asset downloads (spot check)..."
  local download_url
  download_url=$(gh release view "$VERSION" --json 'assets' --jq '.assets[0].url')

  if curl -sILf "$download_url" > /dev/null; then
    echo "Assets are downloadable"
  else
    echo "Warning: Could not verify asset download URL"
  fi

  local expected_count
  expected_count=$(get_total_file_count)
  local binary_count
  binary_count=$(get_binary_count)

  echo ""
  echo "All required assets verified!"
  echo "Expected files: $expected_count ($binary_count binaries + archives + checksums)"
  echo "Actual files: $asset_count"

  if [ "$asset_count" -lt "$expected_count" ]; then
    echo "Warning: Expected $expected_count files, found $asset_count"
  fi
}

main "$@"
