#!/usr/bin/env bash

set -euo pipefail

# Lint the tree, incrementally where a previous result still applies.

: "${GITHUB_REF:?GITHUB_REF is not set}"

# An incremental run compares against main, so it can only skip work for a
# branch whose lint config matches the one main was last linted with.
function needs_full_lint {
	if [[ "$GITHUB_REF" != refs/heads/* ]]; then
		return 0
	fi

	if [[ "$GITHUB_REF" == "refs/heads/main" ]]; then
		return 0
	fi

	git diff --name-only origin/main...HEAD | grep -q '^\.golangci\.yml$'
}

if needs_full_lint; then
	make run-lint
	exit 0
fi

make run-lint-incremental
