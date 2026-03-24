#!/usr/bin/env bash

set -euo pipefail

TAG_NAME="${TAG_NAME:?Required environment variable TAG_NAME}"
OUTPUT_DIR="${1:-previous}"

mkdir -p "$OUTPUT_DIR"

PREV_TAG=$(gh release list --exclude-drafts --exclude-pre-releases -L 50 --json tagName \
	| jq -r --arg tag "${TAG_NAME}" '[.[].tagName] as $tags | ($tags | index($tag)) as $i | if $i == null or ($i + 1) >= ($tags | length) then empty else $tags[$i + 1] end')

if [[ -n "$PREV_TAG" ]]; then
	echo "Previous release: $PREV_TAG"
	if gh release download "$PREV_TAG" -p "coverage-summary.json" -D "$OUTPUT_DIR/" 2>/dev/null; then
		echo "has_previous=true"
	else
		echo "No coverage data in previous release $PREV_TAG"
		echo "has_previous=false"
	fi
else
	echo "No previous release found"
	echo "has_previous=false"
fi
