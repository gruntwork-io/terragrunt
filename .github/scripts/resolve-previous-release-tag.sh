#!/usr/bin/env bash

set -euo pipefail

# Emits `tag=<previous non-prerelease tag>` to $GITHUB_OUTPUT.
# Used by the announce-release workflow to determine the commit range that
# shipped in the current release so issue authors can be notified.

REPO="${REPO:?Required environment variable REPO}"
TAG_NAME="${TAG_NAME:?Required environment variable TAG_NAME}"
GITHUB_OUTPUT="${GITHUB_OUTPUT:?Required environment variable GITHUB_OUTPUT}"

PREVIOUS_TAG=$(
	gh -R "$REPO" release list \
		--exclude-pre-releases --exclude-drafts \
		--limit 50 --json tagName,publishedAt |
		jq -r --arg tag "$TAG_NAME" '
			[sort_by(.publishedAt) | reverse | .[].tagName] as $tags
			| ($tags | index($tag)) as $i
			| if $i == null then empty else ($tags[$i + 1] // empty) end
		'
)

if [[ -z "$PREVIOUS_TAG" ]]; then
	echo "Could not resolve a previous non-prerelease tag for $TAG_NAME" >&2
	exit 1
fi

echo "tag=$PREVIOUS_TAG" >>"$GITHUB_OUTPUT"
