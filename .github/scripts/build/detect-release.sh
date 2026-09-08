#!/usr/bin/env bash

set -euo pipefail

# Record whether this build signs its binaries, for the signing jobs to read.

: "${GITHUB_OUTPUT:?GITHUB_OUTPUT is not set}"

readonly is_release="${IS_RELEASE_INPUT:-false}"

if [[ "$is_release" == "true" ]]; then
	echo "is_release=true" >>"$GITHUB_OUTPUT"
	echo "This is a RELEASE build (from caller input) - signing will be enabled"
	exit 0
fi

echo "is_release=false" >>"$GITHUB_OUTPUT"
echo "This is NOT a release build - signing will be skipped"
