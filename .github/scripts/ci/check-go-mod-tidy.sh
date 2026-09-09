#!/usr/bin/env bash

set -euo pipefail

# Fail if `go mod tidy` would change go.mod or go.sum.

go mod tidy

if git diff --exit-code go.mod go.sum; then
	echo "go.mod and go.sum are tidy"
	exit 0
fi

echo "::error::go.mod or go.sum are not tidy. Please run 'go mod tidy' locally and commit the changes."
exit 1
