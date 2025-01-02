#!/bin/sh

dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

echo "The script is running in the directory $PWD. The first arg is $1. The second arg is $2"

if [ "$TF_VAR_foo" != "FOO" ]
then
    echo "error: TF_VAR_foo is not defined"
    exit 1
fi

if [ "$TF_VAR_bar" != "BAR" ]
then
    echo "error: TF_VAR_bar is not defined"
    exit 1
fi
