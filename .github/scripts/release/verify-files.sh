#!/usr/bin/env bash

set -euo pipefail

# Script to verify all required files are present before upload
# Usage: verify-files.sh <bin-directory>

# Source configuration library
# shellcheck source=lib-release-config.sh
source "$(dirname "$0")/lib-release-config.sh"

function main {
	local -r bin_dir="${1:-bin}"

	if [[ ! -d "$bin_dir" ]]; then
		echo "ERROR: Directory $bin_dir does not exist" >&2
		exit 1
	fi

	verify_config_file

	echo "Verifying required files..."

	# Get all binaries from configuration
	local binaries
	mapfile -t binaries < <(get_all_binaries)

	# Check each binary
	for file in "${binaries[@]}"; do
		if [[ -f "$bin_dir/$file" ]]; then
			echo "$file present"
		else
			echo "$file missing" >&2
			exit 1
		fi
	done

	# Check additional files from configuration
	local additional_files
	mapfile -t additional_files < <(get_additional_files)

	for file in "${additional_files[@]}"; do
		if [[ -f "$bin_dir/$file" ]]; then
			echo "$file present"
		else
			echo "$file missing" >&2
			exit 1
		fi
	done

	echo "All required files verified"

	return 0
}

main "$@"
