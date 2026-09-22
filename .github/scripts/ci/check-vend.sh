#!/usr/bin/env bash

set -euo pipefail

# Fail if internal/vendored/opentofu differs from what vend generates.

go run ./internal/vendored/opentofu/vend

changes="$(git status --porcelain -- internal/vendored/opentofu go.mod go.sum)"

if [[ -z "$changes" ]]; then
	echo "internal/vendored/opentofu matches what vend generates"
	exit 0
fi

echo "$changes"
echo "::error::internal/vendored/opentofu does not match what vend generates. Please run 'go run ./internal/vendored/opentofu/vend' locally and commit the changes." >&2
exit 1
