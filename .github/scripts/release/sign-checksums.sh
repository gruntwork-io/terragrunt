#!/bin/bash

set -e

# Script to sign SHA256SUMS with GPG and Cosign
# Usage: sign-checksums.sh <bin-directory>
#
# Environment variables:
#   GPG_FINGERPRINT - GPG key fingerprint for signing (required)
#
# Outputs:
#   SHA256SUMS.gpgsig - GPG detached signature
#   SHA256SUMS.sig    - Cosign signature
#   SHA256SUMS.pem    - Cosign certificate

function main {
  local -r bin_dir="${1:-bin}"

  if [[ ! -d "$bin_dir" ]]; then
    echo "ERROR: Directory $bin_dir does not exist"
    exit 1
  fi

  if [[ -z "${GPG_FINGERPRINT}" ]]; then
    echo "ERROR: GPG_FINGERPRINT environment variable is not set"
    exit 1
  fi

  # Use pushd/popd to avoid side effects on caller's working directory
  pushd "$bin_dir" || exit 1

  if [[ ! -f "SHA256SUMS" ]]; then
    echo "ERROR: SHA256SUMS file not found in $bin_dir"
    popd || exit 1
    exit 1
  fi

  # GPG signing
  echo "Signing SHA256SUMS with GPG..."
  gpg --batch --yes -u "${GPG_FINGERPRINT}" \
      --output SHA256SUMS.gpgsig \
      --detach-sign SHA256SUMS

  echo "GPG signature created: SHA256SUMS.gpgsig"

  # Cosign signing (keyless OIDC)
  echo "Signing SHA256SUMS with Cosign..."
  cosign sign-blob SHA256SUMS \
      --oidc-issuer=https://token.actions.githubusercontent.com \
      --output-certificate=SHA256SUMS.pem \
      --output-signature=SHA256SUMS.sig \
      --yes

  echo "Cosign signature created: SHA256SUMS.sig"
  echo "Cosign certificate created: SHA256SUMS.pem"

  echo ""
  echo "All signatures generated successfully:"
  ls -la SHA256SUMS*

  popd || exit 1
}

main "$@"
