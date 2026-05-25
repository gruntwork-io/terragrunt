#!/usr/bin/env bash

# Records each invocation as a JSON line in calls.jsonl in this script's
# directory. Tests read calls.jsonl to assert how many times the
# --auth-provider-cmd was invoked and the working directory of each call.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_FILE="${SCRIPT_DIR}/calls.jsonl"

# JSON escape: replace backslash and double-quote.
escaped_pwd="${PWD//\\/\\\\}"
escaped_pwd="${escaped_pwd//\"/\\\"}"

ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

printf '{"ts":"%s","working_dir":"%s","pid":%d}\n' "$ts" "$escaped_pwd" "$$" >>"$LOG_FILE"

printf '{"envs":{"AUTH_PROVIDER_CALL_COUNT_VAR":"ok"}}\n'
