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
  if ! curl -L -o gon.zip "$download_url"; then
    echo "ERROR: Failed to download gon from $download_url"
    rm -f gon.zip
    exit 1
  fi

  # Extract to specific target
  echo "Extracting gon binary..."
  if ! unzip -o gon.zip -d . gon; then
    echo "ERROR: Failed to extract gon from gon.zip"
    rm -f gon.zip
    exit 1
  fi

  # Verify extracted binary exists and is a regular file
  if [[ ! -f ./gon ]]; then
    echo "ERROR: Expected file './gon' not found after extraction"
    rm -f gon.zip
    exit 1
  fi

  # Make executable
  echo "Setting executable permissions..."
  if ! chmod +x ./gon; then
    echo "ERROR: Failed to set executable permissions on ./gon"
    rm -f gon.zip ./gon
    exit 1
  fi

  # Verify it's executable
  if [[ ! -x ./gon ]]; then
    echo "ERROR: File ./gon is not executable after chmod"
    rm -f gon.zip ./gon
    exit 1
  fi

  # Move to system path
  echo "Moving gon to /usr/local/bin/"
  if ! sudo mv ./gon /usr/local/bin/gon; then
    echo "ERROR: Failed to move gon to /usr/local/bin/"
    rm -f gon.zip ./gon
    exit 1
  fi

  if ! sudo chmod +x /usr/local/bin/gon; then
    echo "ERROR: Failed to set executable permissions on /usr/local/bin/gon"
    exit 1
  fi

  # Verify installation
  echo "Verifying gon installation..."
  if ! gon --version; then
    echo "ERROR: gon --version failed after installation"
    exit 1
  fi

  # Cleanup
  rm -f gon.zip

  echo "gon installed successfully"
}

main "$@"
