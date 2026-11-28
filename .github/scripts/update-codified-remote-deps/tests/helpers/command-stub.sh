#!/usr/bin/env bash
# Test stub for `mise` and `git`.
#
# Behavior is driven by env vars set per-test:
#   STUB_RESPONSE  - string written to stdout (default: empty)
#   STUB_EXIT      - exit code (default: 0)
#   STUB_LOG       - if set, append invocation args to this file
#
# Install by copying into a directory on PATH under the command's name.

set -euo pipefail

if [[ -n "${STUB_LOG:-}" ]]; then
	printf '%s\n' "$*" >>"$STUB_LOG"
fi

if [[ -n "${STUB_RESPONSE:-}" ]]; then
	printf '%s\n' "$STUB_RESPONSE"
fi

exit "${STUB_EXIT:-0}"
