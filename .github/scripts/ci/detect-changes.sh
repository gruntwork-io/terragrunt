#!/usr/bin/env bash

set -euo pipefail

# Decide which CI jobs a push needs from the files it changes, and print
# `code=<bool>` and `install=<bool>` for ci.yml to append to $GITHUB_OUTPUT.
#
# Usage: detect-changes.sh <before-sha>
#
# On main the diff starts at <before-sha>, the tip before this push. Anywhere
# else it starts at the merge base with origin/main, so a branch that changed
# code in any commit keeps running the full suite after a docs-only follow-up.
#
# code is false only when every changed file is documentation that no Go test
# or integration fixture reads. install is true when the install script, its
# test, or the workflows that run it changed; that test exercises published
# releases, so no source change can break it. Any doubt about the base, or an
# empty diff, runs everything.

readonly before="${1:-}"

# Documentation paths that nothing but the docs site reads. docs/src/fixtures
# is exercised by the AWS docs integration tests, and docs/tests and
# docs/public/install by the install script tests.
readonly DOCS_PATTERN='^(docs/|[^/]+\.md$)'
readonly DOCS_TESTED_PATTERN='^docs/(src/fixtures/|tests/|public/install$)'
readonly INSTALL_PATTERN='^(docs/public/install|docs/tests/install_test\.sh|\.github/workflows/(ci|install-script-test)\.yml)$'

function main {
	local base changed

	base="$(resolve_base)"

	if [[ -z "$base" ]]; then
		echo "Could not resolve a base commit; running every job" >&2
		print_outputs true true
		return 0
	fi

	changed="$(git diff --name-only "$base" HEAD)"

	if [[ -z "$changed" ]]; then
		echo "No changes against ${base}; running every job" >&2
		print_outputs true true
		return 0
	fi

	print_outputs "$(touches_code "$changed")" "$(touches_install "$changed")"

	return 0
}

# Print the commit the diff starts from, or nothing when it cannot be trusted.
function resolve_base {
	if [[ "${GITHUB_REF:-}" != "refs/heads/main" ]]; then
		git merge-base origin/main HEAD 2>/dev/null || true
		return 0
	fi

	# A branch creation or a force push leaves no usable previous tip.
	if [[ -z "$before" || "$before" =~ ^0+$ ]]; then
		return 0
	fi

	if ! git merge-base --is-ancestor "$before" HEAD 2>/dev/null; then
		return 0
	fi

	echo "$before"

	return 0
}

function touches_code {
	local -r changed="$1"

	if grep -qvE "$DOCS_PATTERN" <<<"$changed" || grep -qE "$DOCS_TESTED_PATTERN" <<<"$changed"; then
		echo true
		return 0
	fi

	echo false

	return 0
}

function touches_install {
	local -r changed="$1"

	if grep -qE "$INSTALL_PATTERN" <<<"$changed"; then
		echo true
		return 0
	fi

	echo false

	return 0
}

function print_outputs {
	local -r code="$1"
	local -r install="$2"

	echo "code=${code}"
	echo "install=${install}"

	return 0
}

main
