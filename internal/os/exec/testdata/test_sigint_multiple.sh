#!/usr/bin/env bash

set -e

INT_REQUIRED=$1
READY_FILE=$2
INT_COUNTER=0

trap int_handler INT

# shellcheck disable=SC2329  # invoked indirectly via trap
function int_handler() {
	INT_COUNTER=$((INT_COUNTER + 1))
	printf '%s\n' "$INT_COUNTER" >"$READY_FILE"
}

# A readable count tells the test the INT trap is installed and, from then on, how many
# signals have actually been handled.
printf '%s\n' "$INT_COUNTER" >"$READY_FILE"

while [[ $INT_COUNTER -lt $INT_REQUIRED ]]; do
	sleep 0.1
done

exit "$INT_COUNTER"
