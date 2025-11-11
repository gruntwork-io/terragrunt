#!/bin/bash

set -e

# Script to sign macOS binaries using gon and Apple notarization
# Usage: sign-macos-binaries.sh <bin-dir>
# Environment variables:
#   AC_PASSWORD: Apple Connect password
#   AC_PROVIDER: Apple Connect provider
#   AC_USERNAME: Apple Connect username
#   MACOS_CERTIFICATE: macOS certificate in P12 format (base64 encoded)
#   MACOS_CERTIFICATE_PASSWORD: Certificate password

function main {
  local -r bin_dir="${1:-bin}"

  # Validate required environment variables
  : "${AC_PASSWORD:?ERROR: AC_PASSWORD is a required environment variable}"
  : "${AC_PROVIDER:?ERROR: AC_PROVIDER is a required environment variable}"
  : "${AC_USERNAME:?ERROR: AC_USERNAME is a required environment variable}"
  : "${MACOS_CERTIFICATE:?ERROR: MACOS_CERTIFICATE is a required environment variable}"
  : "${MACOS_CERTIFICATE_PASSWORD:?ERROR: MACOS_CERTIFICATE_PASSWORD is a required environment variable}"

  if [[ ! -d "$bin_dir" ]]; then
    echo "ERROR: Directory $bin_dir does not exist"
    exit 1
  fi

  echo "Signing macOS binaries..."

  # Sign amd64 binary
  echo "Signing amd64 binary..."
  .github/scripts/setup/mac-sign.sh .gon_amd64.hcl

  # Sign arm64 binary
  echo "Signing arm64 binary..."
  .github/scripts/setup/mac-sign.sh .gon_arm64.hcl

  echo "Done signing the binaries"

  # Remove old unsigned binaries from bin directory
  echo "Removing unsigned binaries from $bin_dir..."
  rm -f "$bin_dir/terragrunt_darwin_amd64"
  rm -f "$bin_dir/terragrunt_darwin_arm64"
  echo "Unsigned binaries removed"

  # Unzip the signed binaries
  echo "Extracting signed binaries..."

  if [[ -f terragrunt_darwin_amd64.zip ]]; then
    echo "Found terragrunt_darwin_amd64.zip, extracting..."
    unzip -o terragrunt_darwin_amd64.zip
    if [[ -f terragrunt_darwin_amd64 ]]; then
      echo "Extracted binary exists, checking signature before move..."
      codesign -dv --verbose=4 terragrunt_darwin_amd64 2>&1 || echo "WARNING: Extracted binary signature check failed"
      mv terragrunt_darwin_amd64 "$bin_dir/"
      echo "Moved signed amd64 binary to $bin_dir/"
    else
      echo "ERROR: Failed to extract terragrunt_darwin_amd64 from ZIP"
      exit 1
    fi
  else
    echo "ERROR: terragrunt_darwin_amd64.zip not found"
    exit 1
  fi

  if [[ -f terragrunt_darwin_arm64.zip ]]; then
    echo "Found terragrunt_darwin_arm64.zip, extracting..."
    unzip -o terragrunt_darwin_arm64.zip
    if [[ -f terragrunt_darwin_arm64 ]]; then
      echo "Extracted binary exists, checking signature before move..."
      codesign -dv --verbose=4 terragrunt_darwin_arm64 2>&1 || echo "WARNING: Extracted binary signature check failed"
      mv terragrunt_darwin_arm64 "$bin_dir/"
      echo "Moved signed arm64 binary to $bin_dir/"
    else
      echo "ERROR: Failed to extract terragrunt_darwin_arm64 from ZIP"
      exit 1
    fi
  else
    echo "ERROR: terragrunt_darwin_arm64.zip not found"
    exit 1
  fi

  # Verify signatures
  echo "Verifying signatures..."

  if [[ -f "$bin_dir/terragrunt_darwin_amd64" ]]; then
    echo "Verifying amd64 signature..."
    codesign -dv --verbose=4 "$bin_dir/terragrunt_darwin_amd64"
  else
    echo "ERROR: $bin_dir/terragrunt_darwin_amd64 not found"
    exit 1
  fi

  if [[ -f "$bin_dir/terragrunt_darwin_arm64" ]]; then
    echo "Verifying arm64 signature..."
    codesign -dv --verbose=4 "$bin_dir/terragrunt_darwin_arm64"
  else
    echo "ERROR: $bin_dir/terragrunt_darwin_arm64 not found"
    exit 1
  fi

  echo "All macOS binaries signed and verified successfully"
}

main "$@"
