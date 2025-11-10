#!/bin/bash

set -e

# Script to generate SHA256 checksums for all release files
# Usage: generate-checksums.sh <bin-directory>

function main {
  local -r bin_dir="${1:-bin}"

  if [[ ! -d "$bin_dir" ]]; then
    echo "ERROR: Directory $bin_dir does not exist"
    exit 1
  fi

  cd "$bin_dir"

  # Generate checksums for all files including individual ZIPs and TAR.GZ archives
  sha256sum terragrunt_* > SHA256SUMS

  echo "SHA256SUMS generated:"
  cat SHA256SUMS

  echo ""
  echo "Total files with checksums: $(wc -l < SHA256SUMS)"
}

main "$@"
