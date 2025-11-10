#!/bin/bash

set -e

# Script to prepare macOS artifacts for signing
# Usage: prepare-macos-artifacts.sh <artifacts-dir> <bin-dir>

function main {
  local -r artifacts_dir="${1:-artifacts}"
  local -r bin_dir="${2:-bin}"

  if [[ ! -d "$artifacts_dir" ]]; then
    echo "ERROR: Artifacts directory $artifacts_dir does not exist"
    exit 1
  fi

  echo "Preparing macOS build artifacts..."

  # Create bin directory
  mkdir -p "$bin_dir"

  # Copy only macOS artifacts (terragrunt_darwin_*) to bin directory
  find "$artifacts_dir" -type f -name 'terragrunt_darwin_*' -exec cp {} "$bin_dir/" \;

  # Verify we found macOS binaries
  if ! ls "$bin_dir"/terragrunt_darwin_* > /dev/null 2>&1; then
    echo "ERROR: No macOS binaries (terragrunt_darwin_*) found in $artifacts_dir"
    exit 1
  fi

  echo "Binary files to sign:"
  ls -lahrt "$bin_dir"/*
}

main "$@"
