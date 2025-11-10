#!/bin/bash

set -e

# Script to download and install gon binary for macOS code signing
# Usage: install-gon.sh [gon-version]
# Environment variables:
#   GON_VERSION: Version of gon to install (default: v0.0.37)

function main {
  local gon_version="${1:-${GON_VERSION:-v0.0.37}}"

  echo "Installing gon version $gon_version..."

  # Download gon release
  local download_url="https://github.com/Bearer/gon/releases/download/${gon_version}/gon_macos.zip"

  echo "Downloading gon from: $download_url"
  curl -L -o gon.zip "$download_url"

  # Extract
  unzip -o gon.zip

  # Make executable
  chmod +x gon

  # Move to system path
  echo "Moving gon to /usr/local/bin/"
  sudo mv gon /usr/local/bin/gon
  sudo chmod +x /usr/local/bin/gon

  # Verify installation
  echo "Verifying gon installation..."
  gon --version

  # Cleanup
  rm -f gon.zip

  echo "gon installed successfully"
}

main "$@"
