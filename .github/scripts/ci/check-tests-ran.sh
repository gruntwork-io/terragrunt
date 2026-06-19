#!/usr/bin/env bash

set -euo pipefail

# Fail if any test defined in the code base is never executed by a CI job.

# Print the top-level name of every test recorded as run in the CI outputs.
function collect_executed_tests {
	local -r outputs_dir="$1"
	local file

	{
		# Match the name attribute, not classname (go-junit-report emits name="Test" classname="pkg").
		grep -rhoE '<testcase[^>]*>' "$outputs_dir" 2>/dev/null | grep -oE '[[:space:]]name="[^"]+"' | sed -E 's/.*name="([^"]+)".*/\1/' || true

		while IFS= read -r file; do
			jq -r 'select(.Action == "run") | .Test // empty' "$file" 2>/dev/null || true
		done < <(grep -rlE '"Action"[[:space:]]*:' "$outputs_dir" 2>/dev/null || true)
	} | sed -E 's#/.*##' | { grep -E '^Test' || true; } | sort -u
}

# Print every test the packages define under the linter's feature build tags.
function list_defined_tests {
	local -r tags="$1"
	shift
	local -ra packages=("$@")

	local -a args=(test -list '.*')
	[[ -n "$tags" ]] && args+=(-tags "$tags")
	args+=("${packages[@]}")

	go "${args[@]}" 2>/dev/null | { grep -E '^Test' || true; } | sort -u
}

function main {
	if [[ $# -lt 1 ]]; then
		echo "ERROR: outputs directory is required" >&2
		echo "Usage: check-tests-ran.sh <outputs-dir> [packages...]" >&2
		exit 2
	fi

	local -r outputs_dir="$1"
	shift
	local -a packages=("$@")
	[[ ${#packages[@]} -eq 0 ]] && packages=("./...")

	if [[ ! -d "$outputs_dir" ]]; then
		echo "ERROR: outputs directory '$outputs_dir' not found" >&2
		exit 2
	fi

	local tags
	if ! tags="$(make -s print-lint-tags)"; then
		echo "ERROR: could not read build tags from the Makefile" >&2
		exit 2
	fi

	collect_executed_tests "$outputs_dir" >"$TMP_DIR/executed"
	if [[ ! -s "$TMP_DIR/executed" ]]; then
		echo "ERROR: no test outputs found in '$outputs_dir'" >&2
		exit 2
	fi

	list_defined_tests "$tags" "${packages[@]}" >"$TMP_DIR/defined"
	if [[ ! -s "$TMP_DIR/defined" ]]; then
		echo "ERROR: no defined tests found via 'go test -list'" >&2
		exit 2
	fi

	local never_run
	never_run="$(comm -23 "$TMP_DIR/defined" "$TMP_DIR/executed")"

	if [[ -n "$never_run" ]]; then
		echo "::error::test(s) defined in the code base but never executed by any CI job:" >&2
		printf '%s\n' "$never_run" | sed 's/^/  /' >&2
		exit 1
	fi

	echo "OK: every defined test ran in at least one CI job."
}

TMP_DIR="$(mktemp -d)"
readonly TMP_DIR
trap 'rm -rf "$TMP_DIR"' EXIT

main "$@"
