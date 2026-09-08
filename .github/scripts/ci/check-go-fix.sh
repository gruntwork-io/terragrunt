#!/usr/bin/env bash

set -euo pipefail

# Fail if `go fix` would rewrite anything that is not already committed.

if make run-go-fix-check; then
	echo "go fix has nothing to apply"
	exit 0
fi

echo "::error::go fix has pending fixes. Please run 'make run-go-fix' locally and commit the changes."
exit 1
