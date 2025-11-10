#!/bin/bash

set -e

# Script to set execution permissions on binaries
# Usage: set-permissions.sh <bin-directory>

function main {
  local -r bin_dir="${1:-bin}"

  if [[ ! -d "$bin_dir" ]]; then
    echo "ERROR: Directory $bin_dir does not exist"
    exit 1
  fi

  cd "$bin_dir"

  # Set execution permissions on all binaries
  chmod +x terragrunt_darwin_amd64
  chmod +x terragrunt_darwin_arm64
  chmod +x terragrunt_linux_386
  chmod +x terragrunt_linux_amd64
  chmod +x terragrunt_linux_arm64
  chmod +x terragrunt_windows_386.exe
  chmod +x terragrunt_windows_amd64.exe

  echo "Execution permissions set on all binaries"
}

main "$@"
