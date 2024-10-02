#!/bin/bash -e

WAIT_TIME=$1

trap int_handler INT

function int_handler() {
        sleep $WAIT_TIME
        exit $WAIT_TIME
}

while true; do sleep 0.1; done