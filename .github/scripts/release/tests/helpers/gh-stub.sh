#!/usr/bin/env bash
# Test stub for the `gh` CLI.
#
# Behavior is driven by env vars set per-test:
#   GH_STUB_RESPONSE  - string written to stdout (default: empty)
#   GH_STUB_EXIT      - exit code (default: 0)
#   GH_STUB_LOG       - if set, append invocation args to this file
#
# Install by prepending a directory containing this file (named `gh`) to PATH.

set -euo pipefail

if [[ -n "${GH_STUB_LOG:-}" ]]; then
	printf '%s\n' "$*" >>"$GH_STUB_LOG"
fi

if [[ -n "${GH_STUB_RESPONSE:-}" ]]; then
	printf '%s' "$GH_STUB_RESPONSE"
fi

exit "${GH_STUB_EXIT:-0}"
