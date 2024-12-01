#!/bin/bash
# script that will fail before $1 attempts

RETRY_ATTEMPTS="$1"
COUNTER_FILE="attempt_counter.txt"

if [[ ! -f "$COUNTER_FILE" ]]; then
    echo "0" > "$COUNTER_FILE"
fi

CURRENT_COUNT=$(($(cat "$COUNTER_FILE") + 1))

echo "$CURRENT_COUNT" > "$COUNTER_FILE"

echo "Current attempt: $CURRENT_COUNT"

if [ "$CURRENT_COUNT" -eq "$RETRY_ATTEMPTS" ]; then
    echo "Success !"
    echo "0" > "$COUNTER_FILE"
    exit 0
else
    echo "Script error: Attempt $CURRENT_COUNT failed. Will succeed on attempt $RETRY_ATTEMPTS." >&2
    exit 1
fi