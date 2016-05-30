#!/bin/bash
# This script is meant to be run in a CI job to run the automated tests.

set -e

# Go's default test timeout is 10 minutes, after which it unceremoniously kills the entire test run, preventing any
# cleanup from running. To prevent that, we set a higher timeout.
readonly TEST_TIMEOUT="45m"

# Our tests do very little that is CPU intensive and spend the vast majority of their time just waiting for AWS, so
# run as many of them in parallel as we can. Circle CI boxes have 32 cores, so this is just 4 tests per core, which it
# should easily be able to handle if we ever get to the point that we have 128 tests!
readonly TEST_PARALLELISM="128"

# SCRIPT_DIR contains the location of the script you're reading now
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Run all the tests that are not in the vendor directory
cd "$SCRIPT_DIR/.." && go test -v -timeout "$TEST_TIMEOUT" -parallel "$TEST_PARALLELISM" $(glide novendor)