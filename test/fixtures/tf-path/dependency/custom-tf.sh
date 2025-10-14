#!/bin/bash
# This script is used as a custom tofu binary for testing
echo "Custom TF script used in $PWD!" >&2
tofu "$@"
