#!/bin/bash

set -e

# Script to verify all expected binaries were downloaded
# Usage: verify-binaries-downloaded.sh <bin-directory> [expected-count]

function main {
  local -r bin_dir="${1:-bin}"
  local -r expected_count="${2:-7}"

  if [[ ! -d "$bin_dir" ]]; then
    echo "ERROR: Directory $bin_dir does not exist"
    exit 1
  fi

  echo "Downloaded binaries:"
  ls -lahrt "$bin_dir"/*

  # Count binaries
  local binary_count
  binary_count=$(find "$bin_dir/" -type f | wc -l)
  echo "Total binaries: $binary_count"

  # Verify expected count
  echo "Expected: at least $expected_count binaries (2 darwin, 3 linux, 2 windows)"

  if [ "$binary_count" -lt "$expected_count" ]; then
    echo "ERROR: Expected at least $expected_count binaries, found $binary_count"
    exit 1
  fi

  echo "✓ All binaries present ($binary_count files)"
}

main "$@"
