#!/usr/bin/env bash

set -euo pipefail

TAG_NAME="${TAG_NAME:?Required environment variable TAG_NAME}"
OUTPUT_DIR="${1:-previous}"

mkdir -p "$OUTPUT_DIR"

# Paginate all releases to ensure TAG_NAME is in the list
ALL_TAGS=$(gh api --paginate "repos/{owner}/{repo}/releases" \
	--jq '[.[] | select(.draft == false and .prerelease == false) | .tag_name]' \
	| jq -s 'add // []')

PREV_TAG=$(jq -r --arg tag "$TAG_NAME" '
	index($tag) as $i |
	if $i == null then "TAG_NOT_FOUND"
	elif ($i + 1) >= length then empty
	else .[$i + 1]
	end' <<<"$ALL_TAGS")

if [[ "$PREV_TAG" == "TAG_NOT_FOUND" ]]; then
	echo "Warning: TAG_NAME '${TAG_NAME}' not found in release list"
	echo "has_previous=false"
	exit 0
fi

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
