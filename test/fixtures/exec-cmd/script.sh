#!/usr/bin/env bash

# Required environment variables:
# TF_VAR_foo - Should be set to "FOO"
# TF_VAR_bar - Should be set to "BAR"

echo "The first arg is $1. The second arg is $2. The script is running in the directory $PWD"

if [ "$TF_VAR_foo" != "FOO" ]
then
    echo "error: TF_VAR_foo must be set to 'FOO' (current value: ${TF_VAR_foo:-not set})"
    exit 1
fi

if [ "$TF_VAR_bar" != "BAR" ]
then
    echo "error: TF_VAR_bar must be set to 'BAR' (current value: ${TF_VAR_bar:-not set})"
    exit 1
fi
