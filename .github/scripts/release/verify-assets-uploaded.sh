#!/bin/bash

set -e

# Script to verify all assets were uploaded to the GitHub release
# Usage: verify-assets-uploaded.sh <bin-directory>
# Environment variables:
#   VERSION: The version/tag to verify
#   GH_TOKEN: GitHub token for authentication

readonly MAX_RETRIES=10

function main {
  local -r bin_dir="${1:-bin}"

  assert_env_var_not_empty "VERSION"
  assert_env_var_not_empty "GH_TOKEN"

  echo "Verifying all assets are accessible..."

  # Get list of assets in the release
  local assets
  assets=$(gh release view "$VERSION" --json assets --jq '.assets[].name')

  local asset_count
  asset_count=$(echo "$assets" | wc -l)

  echo "Found $asset_count assets in release"

  # Expected files (binaries + individual ZIPs + individual TAR.GZ + SHA256SUMS)
  local expected_files=(
    # Individual binaries
    "terragrunt_darwin_amd64"
    "terragrunt_darwin_arm64"
    "terragrunt_linux_386"
    "terragrunt_linux_amd64"
    "terragrunt_linux_arm64"
    "terragrunt_windows_386.exe"
    "terragrunt_windows_amd64.exe"
    # Individual ZIP archives
    "terragrunt_darwin_amd64.zip"
    "terragrunt_darwin_arm64.zip"
    "terragrunt_linux_386.zip"
    "terragrunt_linux_amd64.zip"
    "terragrunt_linux_arm64.zip"
    "terragrunt_windows_386.exe.zip"
    "terragrunt_windows_amd64.exe.zip"
    # Individual TAR.GZ archives
    "terragrunt_darwin_amd64.tar.gz"
    "terragrunt_darwin_arm64.tar.gz"
    "terragrunt_linux_386.tar.gz"
    "terragrunt_linux_amd64.tar.gz"
    "terragrunt_linux_arm64.tar.gz"
    "terragrunt_windows_386.exe.tar.gz"
    "terragrunt_windows_amd64.exe.tar.gz"
    # Checksums
    "SHA256SUMS"
  )

  # Check each expected file
  for expected_file in "${expected_files[@]}"; do
    echo "Checking $expected_file..."

    # Check if file exists in release
    if ! echo "$assets" | grep -q "^${expected_file}$"; then
      echo "$expected_file not found in release, uploading..."

      # Upload the missing file
      if [ -f "$bin_dir/$expected_file" ]; then
        local i
        for ((i=0; i<MAX_RETRIES; i++)); do
          if gh release upload "$VERSION" "$bin_dir/$expected_file" --clobber; then
            echo "Uploaded $expected_file"
            break
          else
            echo "Upload attempt $((i+1))/$MAX_RETRIES failed"
            sleep 5
          fi
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
  download_url=$(gh release view "$VERSION" --json assets --jq '.assets[0].url')

  if curl -sILf "$download_url" > /dev/null; then
    echo "Assets are downloadable"
  else
    echo "Warning: Could not verify asset download URL"
  fi

  echo ""
  echo "All required assets verified!"
  echo "Expected files: 22 (7 binaries + 7 ZIPs + 7 TAR.GZ + SHA256SUMS)"
  echo "Actual files: $asset_count"

  if [ "$asset_count" -lt 22 ]; then
    echo "Warning: Expected 22 files, found $asset_count"
  fi
}

function assert_env_var_not_empty {
  local -r var_name="$1"
  local -r var_value="${!var_name}"

  if [[ -z "$var_value" ]]; then
    echo "ERROR: Required environment variable $var_name not set."
    exit 1
  fi
}

main "$@"
