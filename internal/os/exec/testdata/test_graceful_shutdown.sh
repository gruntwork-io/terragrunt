#!/bin/bash -e

# This script traps SIGINT and exits with code 42 when received.
# It exits with code 1 if terminated by SIGKILL (or any other unexpected termination).
# This is used to verify that the graceful shutdown sends SIGINT rather than SIGKILL.

trap 'exit 42' INT

while true; do sleep 0.1; done
