#!/usr/bin/env bash
# Wrapper around tofu used by TestStackOutputsParallelFetching to confirm that
# `terragrunt stack output` fetches per-unit outputs concurrently. When invoked
# with the `output` subcommand it coordinates via lock files (same approach as
# fixtures/auth-provider-parallel/auth-provider.sh) and logs "Output concurrent"
# when it observes peer invocations. All other subcommands are passed through.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOCK_DIR="${SCRIPT_DIR}/.output-locks"

if [[ "$1" == "output" ]]; then
	mkdir -p "$LOCK_DIR"

	INVOCATION_ID="output-$$-$(date +%s)-$RANDOM"

	touch "${LOCK_DIR}/start-${INVOCATION_ID}"
	echo "Output start ${INVOCATION_ID}" >&2

	WAIT_COUNT=0
	MAX_WAIT=50

	while [[ $WAIT_COUNT -lt $MAX_WAIT ]]; do
		STARTED=$(find "${LOCK_DIR}" -maxdepth 1 -name 'start-*' 2>/dev/null | wc -l | tr -d ' \t')

		if [[ "$STARTED" -ge 2 ]]; then
			echo "Output concurrent ${INVOCATION_ID} detected=$STARTED" >&2
			break
		fi

		sleep 0.01
		WAIT_COUNT=$((WAIT_COUNT + 1))
	done
fi

exec "${REAL_TOFU:-tofu}" "$@"
