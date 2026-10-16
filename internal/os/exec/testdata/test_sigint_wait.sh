#!/usr/bin/env bash

set -e

# The trap sleeps for WAIT_TIME so the caller can see it waited, then exits
# with EXIT_CODE so the caller can tell a handled signal from a kill.
WAIT_TIME=$1
EXIT_CODE=$2
READY_FILE=$3

trap int_handler INT

function int_handler() {
	sleep "$WAIT_TIME"
	exit "$EXIT_CODE"
}

# The marker tells the test the INT trap is installed and a signal can be sent.
: >"$READY_FILE"

while true; do sleep 0.1; done
