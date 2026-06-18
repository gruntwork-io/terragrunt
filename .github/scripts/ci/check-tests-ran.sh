#!/usr/bin/env bash
# Fail if a test defined in the code base is never executed by any CI job: diff the
# tests in the collected CI outputs against `go test -list`. Usage: check-tests-ran.sh <dir> [pkgs...]
set -euo pipefail

outputs_dir="${1:?Usage: check-tests-ran.sh <outputs-dir> [packages...]}"
shift || true
packages=("$@")
[[ ${#packages[@]} -eq 0 ]] && packages=("./...")
[[ -d "$outputs_dir" ]] || {
	echo "Error: outputs dir '$outputs_dir' not found" >&2
	exit 2
}

executed="$(mktemp)"
defined="$(mktemp)"
trap 'rm -f "$executed" "$defined"' EXIT

# Top-level names of tests that actually ran, from JUnit testcases and json run events.
{
	grep -rhoE '<testcase[^>]*name="[^"]+"' "$outputs_dir" 2>/dev/null | sed -E 's/.*name="([^"]+)".*/\1/' || true
	while IFS= read -r f; do
		jq -r 'select(.Action == "run") | .Test // empty' "$f" 2>/dev/null || true
	done < <(grep -rlE '"Action"[[:space:]]*:' "$outputs_dir" 2>/dev/null || true)
} | sed -E 's#/.*##' | { grep -E '^Test' || true; } | sort -u >"$executed"
[[ -s "$executed" ]] || {
	echo "Error: no test outputs found in '$outputs_dir'" >&2
	exit 2
}

tags="$(make -s print-lint-tags)" || {
	echo "Error: could not read build tags from the Makefile" >&2
	exit 2
}
list_args=(test -list '.*')
[[ -n "$tags" ]] && list_args+=(-tags "$tags")
list_args+=("${packages[@]}")
go "${list_args[@]}" 2>/dev/null | { grep -E '^Test' || true; } | sort -u >"$defined"
[[ -s "$defined" ]] || {
	echo "Error: no defined tests found via 'go test -list'" >&2
	exit 2
}

never_run="$(comm -23 "$defined" "$executed")"
[[ -z "$never_run" ]] && {
	echo "OK: every defined test ran in at least one CI job."
	exit 0
}

echo "::error::test(s) defined in the code base but never executed by any CI job:"
printf '%s\n' "$never_run" | sed 's/^/  /'
exit 1
