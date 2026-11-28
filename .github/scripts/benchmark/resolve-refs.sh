#!/usr/bin/env bash

set -euo pipefail

# Resolve the base commit a benchmark run compares HEAD against, and write
# `base_sha`, `head_sha`, and `base_description` to $GITHUB_OUTPUT.
# `base_description` says in Markdown why the base is that commit, such as
# "the latest release, `v1.2.2`".
#
# Usage: resolve-refs.sh <comparison>
#
# <comparison> is one of:
#   release    The latest published release that is not a prerelease and is
#              not HEAD, so that rebuilding the latest release compares it
#              with the one before.
#   self       HEAD, to measure how much two runs of one build differ.
#   custom     $BASE_REF.
#   push       On main, the tip before the push ($BEFORE_SHA), or the parent of
#              HEAD when that tip is unusable. On any other branch, the merge
#              base with origin/main, so the result covers the whole branch.
#
# Env:
#   REPO            owner/name, for release.
#   GH_TOKEN        For release.
#   BASE_REF        Branch, tag, or commit, for custom.
#   BEFORE_SHA      The pushed ref's previous tip, for push.
#   GITHUB_REF      The pushed ref, for push.
#   GITHUB_OUTPUT   Actions output file (defaults to stdout when run locally).

readonly OUTPUT="${GITHUB_OUTPUT:-/dev/stdout}"

function main {
	local -r comparison="${1:?Usage: resolve-refs.sh <release|self|custom|push>}"
	local -r repo="${REPO:-}"
	local -r base_ref="${BASE_REF:-}"
	local -r before_sha="${BEFORE_SHA:-}"
	local -r pushed_ref="${GITHUB_REF:-}"
	local head_sha base base_sha base_description

	head_sha="$(git rev-parse HEAD)"

	case "$comparison" in
	release)
		base="$(resolve_release "${repo:?Required environment variable REPO}" "$head_sha")"
		;;
	self)
		base="$(describe "$head_sha" "the same commit as head, to measure how much two runs of one build differ")"
		;;
	custom)
		base="$(resolve_custom "${base_ref:?Required environment variable BASE_REF}")"
		;;
	push)
		base="$(resolve_push "$pushed_ref" "$before_sha")"
		;;
	*)
		echo "Unknown comparison: ${comparison}" >&2
		return 1
		;;
	esac

	base_sha="${base%%$'\n'*}"
	base_description="${base#*$'\n'}"

	echo "Comparing base ${base_sha} (${base_description}) with head ${head_sha}" >&2

	{
		echo "base_sha=${base_sha}"
		echo "head_sha=${head_sha}"
		echo "base_description=${base_description}"
	} >>"$OUTPUT"

	return 0
}

function resolve_release {
	local -r repo="$1"
	local -r head_sha="$2"
	local tags tag sha

	tags="$(
		gh release list \
			--repo "$repo" \
			--exclude-pre-releases --exclude-drafts \
			--limit 20 --json tagName,publishedAt \
			--jq 'sort_by(.publishedAt) | reverse | .[].tagName'
	)"

	while IFS= read -r tag; do
		if [[ -z "$tag" ]]; then
			continue
		fi

		sha="$(resolve_commit "$tag")"

		if [[ "$sha" != "$head_sha" ]]; then
			describe "$sha" "the latest release, \`${tag}\`"
			return 0
		fi
	done <<<"$tags"

	echo "Could not find a published release of ${repo} to compare with" >&2

	return 1
}

function resolve_push {
	local -r pushed_ref="$1"
	local -r before="$2"

	local sha

	if [[ "$pushed_ref" != "refs/heads/main" ]]; then
		sha="$(git merge-base origin/main HEAD)" || return 1

		describe "$sha" "where this branch leaves \`main\`"
		return 0
	fi

	# A force push leaves a previous tip that HEAD does not descend from.
	if [[ -n "$before" && ! "$before" =~ ^0+$ ]] && git merge-base --is-ancestor "$before" HEAD 2>/dev/null; then
		describe "$before" "the tip of \`main\` before this push"
		return 0
	fi

	sha="$(git rev-parse 'HEAD^')" || return 1

	describe "$sha" "the parent of head"

	return 0
}

function resolve_custom {
	local -r base_ref="$1"
	local sha

	sha="$(resolve_commit "$base_ref")" || return 1

	describe "$sha" "\`${base_ref}\`"

	return 0
}

# Print a base commit and, on the next line, why it is the base.
function describe {
	printf '%s\n%s\n' "$1" "$2"

	return 0
}

# Print the commit a branch, tag, or SHA names. A branch that exists only on the
# remote resolves through origin.
function resolve_commit {
	local -r ref="$1"

	if git rev-parse --verify --quiet "${ref}^{commit}"; then
		return 0
	fi

	if git rev-parse --verify --quiet "origin/${ref}^{commit}"; then
		return 0
	fi

	echo "Could not resolve ${ref} to a commit" >&2

	return 1
}

main "$@"
