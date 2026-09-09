#!/usr/bin/env bash

set -euo pipefail

# Publish the Go cache directories for the actions/cache steps that follow.

: "${GITHUB_OUTPUT:?GITHUB_OUTPUT is not set}"

{
	printf 'go-build=%s\n' "$(go env GOCACHE)"
	printf 'go-mod=%s\n' "$(go env GOMODCACHE)"
	printf 'golangci-lint-cache=%s\n' "${HOME}/.cache/golangci-lint"
} >>"$GITHUB_OUTPUT"
