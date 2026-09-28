#!/usr/bin/env bash

set -e

# This script traps SIGINT and exits with code 42 when received.
# It exits with code 1 if terminated by SIGKILL (or any other unexpected termination).
# This is used to verify that the graceful shutdown sends SIGINT rather than SIGKILL.

READY_FILE=$1

trap 'exit 42' INT

# The marker tells the test the INT trap is installed and a signal can be sent.
: >"$READY_FILE"

while true; do sleep 0.1; done
