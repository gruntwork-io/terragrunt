#!/bin/bash -e

INT_REQUIRED=$1
INT_COUNTER=0

trap int_handler INT

function int_handler() {
    INT_COUNTER=$((INT_COUNTER + 1))
}

while [ $INT_COUNTER -lt $INT_REQUIRED ]
    do sleep 0.1
done

exit $INT_COUNTER