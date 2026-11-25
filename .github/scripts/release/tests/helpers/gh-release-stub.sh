#!/usr/bin/env bash
# Test stub for the `gh release` subcommands, backed by a directory that stands in for the release.
#
# Behavior is driven by env vars set per-test:
#   GH_STUB_RELEASE_DIR      - directory whose files are the release's assets (required)
#   GH_STUB_UPLOAD_FAILURES  - file holding how many uploads fail before one succeeds (default: none fail)
#   GH_STUB_DROP_UPLOADS     - if 'true', uploads exit 0 without storing the asset
#   GH_STUB_VIEW_FAILS       - if 'true', `gh release view` exits 1
#   GH_STUB_LOG              - if set, append invocation args to this file
#
# Install by prepending a directory containing this file (named `gh`) to PATH.

set -euo pipefail

function main {
	: "${GH_STUB_RELEASE_DIR:?ERROR: GH_STUB_RELEASE_DIR is a required environment variable}"

	if [[ -n "${GH_STUB_LOG:-}" ]]; then
		printf '%s\n' "$*" >>"$GH_STUB_LOG"
	fi

	case "$1 $2" in
	"release view")
		if [[ "${GH_STUB_VIEW_FAILS:-false}" == "true" ]]; then
			echo "gh stub: simulated view failure" >&2
			exit 1
		fi

		ls "$GH_STUB_RELEASE_DIR"
		;;
	"release upload")
		upload "$4"
		;;
	"release download")
		# gh release download <tag> --pattern <name> --output -
		cat "$GH_STUB_RELEASE_DIR/$5"
		;;
	*)
		echo "gh stub: unsupported invocation: $*" >&2
		exit 2
		;;
	esac
}

function upload {
	local -r file="$1"

	local failures=0
	if [[ -f "${GH_STUB_UPLOAD_FAILURES:-}" ]]; then
		failures=$(<"$GH_STUB_UPLOAD_FAILURES")
	fi

	if ((failures > 0)); then
		echo $((failures - 1)) >"$GH_STUB_UPLOAD_FAILURES"
		echo "gh stub: simulated upload failure" >&2
		exit 1
	fi

	if [[ "${GH_STUB_DROP_UPLOADS:-false}" == "true" ]]; then
		return 0
	fi

	cp "$file" "$GH_STUB_RELEASE_DIR/"
}

main "$@"
