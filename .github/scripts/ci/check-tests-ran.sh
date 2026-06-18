#!/usr/bin/env bash
# Detect tests that are defined in the code base but never executed by any CI job.
#
# It builds the set of tests that actually ran from the test outputs collected
# across every CI job (JUnit result.xml files and/or `go test -json` event
# files), then compares that set against every test defined in the code. Any
# defined test that no job executed is reported and the script exits non-zero.
#
# This mirrors the ticket's approach: process the CI test outputs and cross-check
# them against the code, rather than statically modelling the workflow matrix.
#
# Usage: check-tests-ran.sh <outputs-dir> [packages...]
#   <outputs-dir>  directory holding the downloaded test-result artifacts
#   [packages...]  Go packages to enumerate defined tests in (default ./...)
set -euo pipefail

outputs_dir="${1:?Usage: check-tests-ran.sh <outputs-dir> [packages...]}"
shift || true
packages=("$@")
[[ ${#packages[@]} -eq 0 ]] && packages=("./...")

if [[ ! -d "$outputs_dir" ]]; then
	echo "Error: outputs dir '$outputs_dir' not found" >&2
	exit 2
fi

executed="$(mktemp)"
defined="$(mktemp)"
trap 'rm -f "$executed" "$defined"' EXIT

# executed_tests prints every top-level test name observed in the outputs.
executed_tests() {
	# JUnit <testcase ... name="TestFoo"> (subtests appear as TestFoo/case).
	grep -rhoE '<testcase[^>]*name="[^"]+"' "$outputs_dir" 2>/dev/null |
		sed -E 's/.*name="([^"]+)".*/\1/' || true

	# `go test -json` run events: {"Action":"run","Test":"TestFoo"}.
	local f
	while IFS= read -r f; do
		jq -r 'select(.Action == "run") | .Test // empty' "$f" 2>/dev/null || true
	done < <(grep -rlE '"Action"[[:space:]]*:' "$outputs_dir" 2>/dev/null || true)
}

# defined_tags prints the comma-separated custom build tags used in the tree,
# excluding GOOS/GOARCH tokens, matching the Makefile LINT_TAGS computation.
defined_tags() {
	local ignore='windows|linux|darwin|freebsd|openbsd|netbsd|dragonfly|solaris|plan9|js|wasip1|aix|android|illumos|ios|386|amd64|arm|arm64|mips|mips64|mips64le|mipsle|ppc64|ppc64le|riscv64|s390x|wasm'
	grep -rh --include='*.go' --exclude-dir='.git' --exclude-dir='.terragrunt-cache' 'go:build' . |
		sed 's/.*go:build[[:space:]]*//' |
		tr -cs '[:alnum:]_' '\n' |
		grep -vE "^(${ignore})$" |
		sed '/^$/d' |
		sort -u |
		paste -sd, -
}

# Strip subtests, keep only top-level Test* names, dedupe.
executed_tests | sed -E 's#/.*##' | { grep -E '^Test' || true; } | sort -u >"$executed"

if [[ ! -s "$executed" ]]; then
	echo "Error: no executed tests found in '$outputs_dir' (no result.xml or json events collected?)" >&2
	exit 2
fi

tags="$(defined_tags)"
list_args=(test -list '.*')
[[ -n "$tags" ]] && list_args+=(-tags "$tags")
list_args+=("${packages[@]}")
go "${list_args[@]}" 2>/dev/null | { grep -E '^Test' || true; } | sort -u >"$defined"

if [[ ! -s "$defined" ]]; then
	echo "Error: no defined tests found via 'go test -list' (build failure?)" >&2
	exit 2
fi

never_run="$(comm -23 "$defined" "$executed")"

if [[ -n "$never_run" ]]; then
	count="$(printf '%s\n' "$never_run" | grep -c .)"
	echo "::error::${count} test(s) are defined in the code base but were never executed by any CI job:"
	printf '%s\n' "$never_run" | sed 's/^/  /'
	echo
	echo "Route each one to a CI job: rename it to match the job's -run filter (e.g. TestAws*/TestGcp*)," >&2
	echo "enable its build tag in a job, or run it in the base suite." >&2
	exit 1
fi

echo "OK: every defined test was executed by at least one CI job."
