#!/usr/bin/env bash

set -e

INT_REQUIRED=$1
READY_FILE=$2
INT_COUNTER=0
PUBLISHED=-1

trap int_handler INT

# shellcheck disable=SC2329  # invoked indirectly via trap
function int_handler() {
	INT_COUNTER=$((INT_COUNTER + 1))
}

# Publishing outside the trap means a count the test can read implies the handler returned.
function publish_count() {
	if [[ $INT_COUNTER -ne $PUBLISHED ]]; then
		printf '%s\n' "$INT_COUNTER" >"$READY_FILE"
		PUBLISHED=$INT_COUNTER
	fi
}

publish_count

while [[ $INT_COUNTER -lt $INT_REQUIRED ]]; do
	sleep 0.1
	publish_count
done

exit "$INT_COUNTER"
