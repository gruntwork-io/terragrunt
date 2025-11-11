#!/bin/bash

set -e

# Script to verify all expected binaries were downloaded
# Usage: verify-binaries-downloaded.sh <bin-directory> [expected-count]

# Source configuration library
# shellcheck source=lib-release-config.sh
source "$(dirname "$0")/lib-release-config.sh"

function resolve_expected_count {
  local -r count_override="$1"

  # If override provided, use it; otherwise get from config
  if [[ -n "$count_override" ]]; then
    echo "$count_override"
    return 0
  fi

  verify_config_file
  get_binary_count
}

function main {
  local -r bin_dir="${1:-bin}"
  local -r count_override="${2:-}"

  local expected_count
  expected_count=$(resolve_expected_count "$count_override")

  if [[ ! -d "$bin_dir" ]]; then
    echo "ERROR: Directory $bin_dir does not exist"
    exit 1
  fi

  # Count binaries first
  local binary_count
  binary_count=$(find "$bin_dir/" -type f | wc -l)

  # List binaries if any exist (resilient to empty directory)
  if [ "$binary_count" -gt 0 ]; then
    echo "Downloaded binaries:"
    ls -lahrt "$bin_dir"/*
  else
    echo "No binaries found in $bin_dir"
  fi

  echo "Total binaries: $binary_count"

  # Verify expected count
  echo "Expected: at least $expected_count binaries"

  if [ "$binary_count" -lt "$expected_count" ]; then
    echo "ERROR: Expected at least $expected_count binaries, found $binary_count"
    exit 1
  fi

  echo "All binaries present ($binary_count files)"
}

main "$@"
