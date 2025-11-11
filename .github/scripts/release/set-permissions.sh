#!/bin/bash

set -e

# Script to set execution permissions on binaries
# Usage: set-permissions.sh <bin-directory>

# Source configuration library
# shellcheck source=lib-release-config.sh
source "$(dirname "$0")/lib-release-config.sh"

function main {
  local -r bin_dir="${1:-bin}"

  if [[ ! -d "$bin_dir" ]]; then
    echo "ERROR: Directory $bin_dir does not exist"
    exit 1
  fi

  verify_config_file

  # Use pushd/popd to avoid side effects on caller's working directory
  pushd "$bin_dir" || return 1

  # Get list of all binaries from configuration
  local binaries
  mapfile -t binaries < <(get_all_binaries)

  # Set execution permissions on all binaries
  for binary in "${binaries[@]}"; do
    if [[ -f "$binary" ]]; then
      chmod +x "$binary"
      echo "Set +x on $binary"
    else
      echo "Warning: Binary $binary not found, skipping"
    fi
  done

  echo "Execution permissions set on all binaries"

  # Return to original directory
  popd || return 1
}

main "$@"
