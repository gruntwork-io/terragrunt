#!/usr/bin/env bash

set -euo pipefail

# Script to upload tip build assets to S3
# Usage: upload-tip-build.sh <bin-directory>
#
# Environment variables:
#   BUCKET     - S3 bucket name (required)
#   PREFIX     - S3 key prefix, e.g. "tip" or "test" (required)
#   COMMIT_SHA - Git commit SHA used as the build ref (required)

function main {
	local -r bin_dir="${1:-bin}"

	if [[ ! -d "$bin_dir" ]]; then
		echo "ERROR: Directory $bin_dir does not exist" >&2
		exit 1
	fi

	if [[ -z "${BUCKET:-}" ]]; then
		echo "ERROR: BUCKET environment variable is not set" >&2
		exit 1
	fi

	if [[ -z "${PREFIX:-}" ]]; then
		echo "ERROR: PREFIX environment variable is not set" >&2
		exit 1
	fi

	if [[ -z "${COMMIT_SHA:-}" ]]; then
		echo "ERROR: COMMIT_SHA environment variable is not set" >&2
		exit 1
	fi

	local -r s3_path="s3://${BUCKET}/${PREFIX}/${COMMIT_SHA}"
	echo "Uploading to ${s3_path}/"

	local pids=()

	# Upload tar.gz archives
	local filename
	for f in "${bin_dir}"/*.tar.gz; do
		filename="$(basename "$f")"
		echo "Uploading ${filename}..."
		aws s3 cp "${bin_dir}/${filename}" "${s3_path}/${filename}" &
		pids+=($!)
	done

	# Upload checksums and signatures
	for f in SHA256SUMS SHA256SUMS.gpgsig SHA256SUMS.sigstore.json; do
		if [[ ! -f "${bin_dir}/${f}" ]]; then
			echo "WARNING: ${f} not found in ${bin_dir}, skipping" >&2
			continue
		fi
		echo "Uploading ${f}..."
		aws s3 cp "${bin_dir}/${f}" "${s3_path}/${f}" &
		pids+=($!)
	done

	# Wait for all uploads and fail if any failed
	local failed=0
	for pid in "${pids[@]}"; do
		if ! wait "$pid"; then
			failed=1
		fi
	done

	if [[ "$failed" -ne 0 ]]; then
		echo "ERROR: One or more uploads failed" >&2
		return 1
	fi

	echo ""
	echo "Upload complete to ${s3_path}/"
	return 0
}

main "$@"
