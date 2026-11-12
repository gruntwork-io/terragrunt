#!/usr/bin/env bash

# Add the minor series of an OpenTofu or Terraform release to the docs
# compatibility table, unless the table already lists it.
#
# The new entry's terragrunt_min is the patch release after the latest
# Terragrunt tag. The docs hide an entry until its terragrunt_min is released.
#
# Usage: add-compatibility-entry.sh <opentofu|terraform> <version>

set -euo pipefail

# shellcheck source=lib.sh
source "$(dirname "$0")/lib.sh"

readonly DATA_FILE="docs/src/data/compatibility/compatibility.json"
readonly REPO="https://github.com/gruntwork-io/terragrunt.git"
readonly VERSION_PATTERN='^[0-9]+\.[0-9]+\.[0-9]+$'
readonly TAG_PATTERN='^[0-9a-f]+\trefs/tags/v\K[0-9]+\.[0-9]+\.[0-9]+$'

main() {
	[[ $# -eq 2 ]] || die "usage: $(basename "$0") <opentofu|terraform> <version>"

	local -r tool="$1" version="$2"
	local id latest terragrunt_min

	require_tools git grep jq mktemp
	require_env GITHUB_OUTPUT

	[[ "$tool" == "opentofu" || "$tool" == "terraform" ]] || die "unexpected tool: $tool"
	[[ "$version" =~ $VERSION_PATTERN ]] || die "unexpected version: $version"

	local -r series="${version%.*}.x"
	id="$tool-$series"
	readonly id

	if jq -e --arg id "$id" 'any(.[]; .id == $id)' "$DATA_FILE" >/dev/null; then
		printf 'note=\n' >>"$GITHUB_OUTPUT"
		printf '%s: %s is already listed\n' "$(basename "$0")" "$id"
		return
	fi

	latest="$(latest_terragrunt)"
	terragrunt_min="${latest%.*}.$((${latest##*.} + 1))"
	readonly latest terragrunt_min

	add_entry "$id" "$tool" "$series" "$terragrunt_min"

	printf "note=Adds \`%s\` to \`%s\` with \`terragrunt_min\` \`%s\`, the patch release after \`v%s\`. Correct it before merging if the next Terragrunt release has a different version.\n" \
		"$id" "$DATA_FILE" "$terragrunt_min" "$latest" >>"$GITHUB_OUTPUT"
	printf '%s: added %s with terragrunt_min %s\n' "$(basename "$0")" "$id" "$terragrunt_min"
}

# latest_terragrunt: echoes the highest X.Y.Z among the Terragrunt release tags.
latest_terragrunt() {
	local tags

	tags="$(git ls-remote --tags --refs --sort=-v:refname "$REPO" 'v*')" || die "cannot list tags of $REPO"

	grep -m1 -oP "$TAG_PATTERN" <<<"$tags" || die "no release tag found in $REPO"
}

# add_entry <id> <tool> <series> <terragrunt_min>: inserts the entry above the
# tool's other entries, with an order one past the tool's highest.
add_entry() {
	local -r id="$1" tool="$2" series="$3" terragrunt_min="$4"
	local updated

	updated="$(mktemp)"

	jq --arg id "$id" --arg tool "$tool" --arg series "$series" --arg min "$terragrunt_min" '
		(map(.tool) | index($tool)) as $at
		| (map(select(.tool == $tool).order) | max + 1) as $order
		| .[:$at] + [{
			id: $id,
			tool: $tool,
			version: $series,
			terragrunt_min: $min,
			terragrunt_max: null,
			order: $order
		}] + .[$at:]
	' "$DATA_FILE" >"$updated"

	mv "$updated" "$DATA_FILE"
}

main "$@"
