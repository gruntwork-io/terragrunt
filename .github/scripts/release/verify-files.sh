#!/bin/bash

set -e

# Script to verify all required files are present before upload
# Usage: verify-files.sh <bin-directory>

function main {
  local -r bin_dir="${1:-bin}"

  if [[ ! -d "$bin_dir" ]]; then
    echo "ERROR: Directory $bin_dir does not exist"
    exit 1
  fi

  echo "Verifying required files..."

  # Check macOS binaries
  for file in terragrunt_darwin_amd64 terragrunt_darwin_arm64; do
    if [ -f "$bin_dir/$file" ]; then
      echo "$file present"
    else
      echo "$file missing"
      exit 1
    fi
  done

  # Check Windows binaries
  for file in terragrunt_windows_amd64.exe terragrunt_windows_386.exe; do
    if [ -f "$bin_dir/$file" ]; then
      echo "$file present"
    else
      echo "$file missing"
      exit 1
    fi
  done

  # Check Linux binaries
  for file in terragrunt_linux_386 terragrunt_linux_amd64 terragrunt_linux_arm64; do
    if [ -f "$bin_dir/$file" ]; then
      echo "$file present"
    else
      echo "$file missing"
      exit 1
    fi
  done

  # Check SHA256SUMS
  if [ -f "$bin_dir/SHA256SUMS" ]; then
    echo "SHA256SUMS present"
  else
    echo "SHA256SUMS missing"
    exit 1
  fi

  echo "All required files verified"
}

main "$@"
