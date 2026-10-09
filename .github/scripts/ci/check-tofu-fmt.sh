#!/usr/bin/env bash

set -euo pipefail

# Fail if any tracked OpenTofu file is not formatted canonically. This is the
# tofu-fmt hook from .pre-commit-config.yaml, run over every tracked file.

# hclvalidate/valid holds configurations that are valid but deliberately
# unformatted, matching the hook's exclude.
readonly EXCLUDE_PATTERN='^test/fixtures/hclvalidate/valid/'

if git ls-files -z '*.tf' '*.tofu' | grep -zv "$EXCLUDE_PATTERN" | xargs -0 tofu fmt -check -diff; then
	echo "OpenTofu files are formatted"
	exit 0
fi

echo "::error::OpenTofu files are not formatted. Please run 'tofu fmt' on the files above and commit the changes." >&2
exit 1
