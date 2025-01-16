#!/usr/bin/env bash

set -euo pipefail

URL="${URL:?Required environment variable URL}"
REPO="${REPO:?Required environment variable REPO}"
TAG_NAME="${TAG_NAME:?Required environment variable TAG_NAME}"
ROLE_ID="${ROLE_ID:?Required environment variable ROLE_ID}"
USERNAME="${USERNAME:?Required environment variable USERNAME}"
AVATAR_URL="${AVATAR_URL:?Required environment variable AVATAR_URL}"

if RELEASE_JSON=$(gh -R "$REPO" release view "$TAG_NAME" --json body --json url --json name); then
	RELEASE_NOTES_LENGTH=$(jq '.body | length' <<<"$RELEASE_JSON")

	if [ "$RELEASE_NOTES_LENGTH" -gt 2000 ]; then
		echo "Release notes are too long ($RELEASE_NOTES_LENGTH characters), truncating to 1997 characters, truncating the last line, then appending '…'"
		RELEASE_JSON=$(jq '.body |= .[:1997]' <<<"$RELEASE_JSON" | jq '. | split("\r\n") | del(.[-1]) | join("\r\n")' | jq '. + "\r\n…"')
	fi

	RELEASE_NOTES=$(jq '.body' <<<"$RELEASE_JSON")

	PAYLOAD=$(
		jq \
			--argjson release_notes "$RELEASE_NOTES" \
			--arg username "$USERNAME" \
			--arg avatar_url "$AVATAR_URL" \
			-cn '{"content": $release_notes, username: $username, avatar_url: $avatar_url, "flags": 4}'
	)

	tmpfile=$(mktemp)
	jq '.content = "'"<@&$ROLE_ID> $(jq -r '.name' <<<"$RELEASE_JSON")\n"'>>> " + .content + "'"\n\n**[View release on GitHub]($(jq -r '.url' <<<"$RELEASE_JSON"))**"'"' <<<"$PAYLOAD" >"$tmpfile"

	curl -X POST \
		--data-binary "@$tmpfile" \
		-H "Content-Type: application/json" \
		"$URL"
fi
