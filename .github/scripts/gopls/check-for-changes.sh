#!/usr/bin/env bash

set -euo pipefail

: "${HAS_FIXES:?}"

if [ "$HAS_FIXES" != "true" ]; then
  echo "has_changes=false" >> "$GITHUB_OUTPUT"
  exit 0
fi

if git diff --staged --quiet; then
  echo "has_changes=false" >> "$GITHUB_OUTPUT"
  exit 0
fi

echo "has_changes=true" >> "$GITHUB_OUTPUT"
