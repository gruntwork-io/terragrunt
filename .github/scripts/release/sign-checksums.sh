#!/usr/bin/env bash

set -euo pipefail

# Script to sign SHA256SUMS with GPG and Cosign
# Usage: sign-checksums.sh <bin-directory>
#
# Environment variables:
#   GPG_FINGERPRINT        - GPG key fingerprint for signing (required)
#   SIGNING_GPG_PASSPHRASE - GPG key passphrase (required)
#
# Outputs:
#   SHA256SUMS.gpgsig - GPG detached signature
#   SHA256SUMS.sigstore.json - Cosign sigstore bundle
#   SHA256SUMS.sig           - Cosign signature (legacy, extracted from bundle)
#   SHA256SUMS.pem           - Cosign certificate (legacy, extracted from bundle)

function main {
	local -r bin_dir="${1:-bin}"

	if [[ ! -d "$bin_dir" ]]; then
		echo "ERROR: Directory $bin_dir does not exist" >&2
		exit 1
	fi

	if [[ -z "${GPG_FINGERPRINT}" ]]; then
		echo "ERROR: GPG_FINGERPRINT environment variable is not set" >&2
		exit 1
	fi

	if [[ -z "${SIGNING_GPG_PASSPHRASE}" ]]; then
		echo "ERROR: SIGNING_GPG_PASSPHRASE environment variable is not set" >&2
		exit 1
	fi

	# Use pushd/popd to avoid side effects on caller's working directory
	pushd "$bin_dir" || exit 1

	if [[ ! -f "SHA256SUMS" ]]; then
		echo "ERROR: SHA256SUMS file not found in $bin_dir" >&2
		popd || exit 1
		exit 1
	fi

	# GPG signing
	echo "Signing SHA256SUMS with GPG..."
	gpg --batch --yes -u "${GPG_FINGERPRINT}" \
		--pinentry-mode loopback \
		--passphrase "${SIGNING_GPG_PASSPHRASE}" \
		--output SHA256SUMS.gpgsig \
		--detach-sign SHA256SUMS

	echo "GPG signature created: SHA256SUMS.gpgsig"

	# Cosign signing (keyless OIDC) - produces sigstore bundle
	echo "Signing SHA256SUMS with Cosign..."
	cosign sign-blob SHA256SUMS \
		--bundle=SHA256SUMS.sigstore.json \
		--yes

	echo "Cosign bundle created: SHA256SUMS.sigstore.json"

	# Extract legacy .sig and .pem from bundle for backward compatibility
	echo "Extracting legacy signature files from bundle..."
	jq -r '.messageSignature.signature' SHA256SUMS.sigstore.json >SHA256SUMS.sig
	jq -r '.verificationMaterial.certificate.rawBytes' SHA256SUMS.sigstore.json |
		base64 --decode |
		openssl x509 -inform DER -outform PEM -out SHA256SUMS.pem

	echo "Cosign signature created: SHA256SUMS.sig"
	echo "Cosign certificate created: SHA256SUMS.pem"

	echo ""
	echo "All signatures generated successfully:"
	ls -la SHA256SUMS*

	popd || exit 1

	return 0
}

main "$@"
