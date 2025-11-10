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

  # Copy all artifacts to bin directory
  find "$artifacts_dir" -type f -exec cp {} "$bin_dir/" \;

  echo "Binary files to sign:"
  ls -lahrt "$bin_dir"/*
}

main "$@"
