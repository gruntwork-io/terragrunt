#!/usr/bin/env bash

set -euo pipefail

# Script to verify all assets were uploaded to the GitHub release
# Usage: verify-assets-uploaded.sh <bin-directory>
# Environment variables:
#   VERSION: The version/tag to verify
#   GH_TOKEN: GitHub token for authentication
#   CLOBBER: Set to 'true' to overwrite existing assets during retry (default: false)

# Source configuration library
# shellcheck source=lib-release-config.sh
source "$(dirname "$0")/lib-release-config.sh"

readonly MAX_RETRIES=10
readonly RETRY_DELAY_SECONDS=5
readonly SPOT_CHECK_FILE="SHA256SUMS"

function main {
	local -r bin_dir="${1:-bin}"

	: "${VERSION:?ERROR: VERSION is a required environment variable}"
	: "${GH_TOKEN:?ERROR: GH_TOKEN is a required environment variable}"
	verify_config_file

	local -a expected_files
	mapfile -t expected_files < <(get_all_expected_files)

	echo "Verifying all assets are present..."
	upload_missing_assets "$bin_dir" "${expected_files[@]}"
	assert_no_missing_assets "${expected_files[@]}"

	echo ""
	echo "Verifying asset download (spot check)..."
	assert_download_matches_local "$bin_dir/$SPOT_CHECK_FILE"

	local asset_count
	asset_count=$(list_assets | wc -l)

	echo ""
	echo "All required assets verified!"
	echo "Expected files: ${#expected_files[@]} ($(get_binary_count) binaries + archives + checksums)"
	echo "Release assets: $asset_count"

	return 0
}

# Upload each expected file that the release lacks
# Usage: upload_missing_assets <bin-directory> <expected-file>...
function upload_missing_assets {
	local -r bin_dir="$1"
	shift

	local missing
	missing=$(missing_assets "$@")

	if [[ -z "$missing" ]]; then
		echo "All $# expected files present"
		return 0
	fi

	local file
	while IFS= read -r file; do
		echo "$file not found in release, uploading..."
		upload_asset "$bin_dir/$file"
	done <<<"$missing"

	return 0
}

# Exit with an error naming each expected file that the release lacks
# Usage: assert_no_missing_assets <expected-file>...
function assert_no_missing_assets {
	local missing
	missing=$(missing_assets "$@")

	if [[ -z "$missing" ]]; then
		return 0
	fi

	local -a missing_files
	mapfile -t missing_files <<<"$missing"

	echo "ERROR: Release $VERSION is missing ${#missing_files[@]} of $# expected files:" >&2
	printf '  %s\n' "${missing_files[@]}" >&2
	exit 1
}

# Exit with an error unless the release's copy of a file downloads and matches the local one
function assert_download_matches_local {
	local -r local_file="$1"
	local -r name="$(basename "$local_file")"

	if ! retry download_matches_local "$local_file"; then
		echo "ERROR: Downloaded $name does not match $local_file" >&2
		exit 1
	fi

	echo "$name downloads and matches the local file"

	return 0
}

# Print each of the given files that is not an asset on the release, one per line
function missing_assets {
	local assets
	assets=$(list_assets) || return 1

	local expected_file
	for expected_file in "$@"; do
		if ! grep -qxF -- "$expected_file" <<<"$assets"; then
			echo "$expected_file"
		fi
	done

	return 0
}

# Print the name of every asset on the release, one per line
function list_assets {
	gh release view "$VERSION" --json 'assets' --jq '.assets[].name'
}

# Upload one local file to the release, retrying on failure
function upload_asset {
	local -r local_file="$1"
	local -r name="$(basename "$local_file")"

	if [[ ! -f "$local_file" ]]; then
		echo "ERROR: File $local_file not found locally" >&2
		exit 1
	fi

	local -a upload_flags=()
	if [[ "${CLOBBER:-false}" == "true" ]]; then
		upload_flags+=(--clobber)
	fi

	if ! retry gh release upload "$VERSION" "$local_file" "${upload_flags[@]}"; then
		echo "ERROR: Failed to upload $name after $MAX_RETRIES attempts" >&2
		exit 1
	fi

	echo "Uploaded $name"

	return 0
}

# Download one asset from the release and compare it to the local file of the same name
function download_matches_local {
	local -r local_file="$1"

	# A draft release serves assets only to authenticated requests, which gh sends.
	gh release download "$VERSION" --pattern "$(basename "$local_file")" --output - |
		cmp -s - "$local_file"
}

# Run a command until it succeeds, up to MAX_RETRIES attempts
function retry {
	local attempt
	for ((attempt = 1; attempt <= MAX_RETRIES; attempt++)); do
		if "$@"; then
			return 0
		fi

		echo "Attempt $attempt/$MAX_RETRIES failed" >&2

		if ((attempt < MAX_RETRIES)); then
			sleep "$RETRY_DELAY_SECONDS"
		fi
	done

	return 1
}

main "$@"
