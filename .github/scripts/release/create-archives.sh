#!/bin/bash

set -e

# Script to create ZIP and TAR.GZ archives for each binary
# Usage: create-archives.sh <bin-directory>

function main {
  local -r bin_dir="${1:-bin}"

  if [[ ! -d "$bin_dir" ]]; then
    echo "ERROR: Directory $bin_dir does not exist"
    exit 1
  fi

  # Use pushd/popd to avoid side effects on caller's working directory
  pushd "$bin_dir" || return 1

  echo "Creating individual archives for each binary..."

  # Create individual ZIP and TAR.GZ archives for each binary (preserving execute permissions)
  for binary in terragrunt_*; do
    # Skip if it's already an archive file
    if [[ "$binary" == *.zip ]] || [[ "$binary" == *.tar.gz ]]; then
      continue
    fi

    # Create ZIP archive
    zip "$binary.zip" "$binary"
    echo "Created: $binary.zip"

    # Create TAR.GZ archive (preserves Unix permissions including +x)
    tar -czf "$binary.tar.gz" "$binary"
    echo "Created: $binary.tar.gz"
  done

  echo ""
  echo "All individual archives created:"
  echo "ZIP archives:"
  ls -lh *.zip
  echo ""
  echo "TAR.GZ archives:"
  ls -lh *.tar.gz

  # Return to original directory
  popd || return 1
}

main "$@"
