#!/usr/bin/env bash

set -euo pipefail

# Script to import a base64-encoded GPG private key and export the
# public key for release artifact verification.
#
# Usage: import-gpg-key.sh <output_dir>
# Environment variables:
#   SIGNING_GPG_PRIVATE_KEY: Base64-encoded GPG private key
#   GITHUB_ENV: Path to GitHub environment file

function main {
	local output_dir="${1:?ERROR: output directory is required as first argument}"

	: "${SIGNING_GPG_PRIVATE_KEY:?ERROR: SIGNING_GPG_PRIVATE_KEY is a required environment variable}"
	: "${GITHUB_ENV:?ERROR: GITHUB_ENV is a required environment variable}"

	base64 --decode <<<"${SIGNING_GPG_PRIVATE_KEY}" | gpg --batch --import

	local gpg_fingerprint
	gpg_fingerprint=$(gpg --list-secret-keys --keyid-format LONG | awk '/^sec/{sub(/.*\//, "", $2); print $2; exit}')

	printf 'GPG_FINGERPRINT=%s\n' "${gpg_fingerprint}" >>"${GITHUB_ENV}"
	gpg --armor --export "${gpg_fingerprint}" >"${output_dir}/terragrunt-signing-key.asc"

	printf 'Imported GPG key: %s\n' "${gpg_fingerprint}"

	return 0
}

main "$@"
