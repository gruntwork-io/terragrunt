#!/usr/bin/env bash

set -e

WAIT_TIME=$1
READY_FILE=$2

trap int_handler INT

function int_handler() {
	sleep "$WAIT_TIME"
	exit "$WAIT_TIME"
}

# The marker tells the test the INT trap is installed and a signal can be sent.
: >"$READY_FILE"

while true; do sleep 0.1; done
