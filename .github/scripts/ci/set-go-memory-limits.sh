#!/usr/bin/env bash

set -euo pipefail

# Cap the Go heap for the lint job. Linting every build tag at the GOGC=400 the
# workflow sets grows the heap past the runner's physical memory, and the runner
# answers with a shutdown signal.

: "${GITHUB_ENV:?GITHUB_ENV is not set}"

{
	echo "GOGC=100"
	echo "GOMEMLIMIT=20GiB"
} >>"$GITHUB_ENV"
