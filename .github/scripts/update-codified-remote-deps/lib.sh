#!/usr/bin/env bash

# Helpers shared by the update-codified-remote-deps check scripts.
#
# Usage: source "$(dirname "$0")/lib.sh"

# require_tools <tool>...
require_tools() {
	local tool

	for tool in "$@"; do
		command -v "$tool" >/dev/null 2>&1 || die "$tool is required but not installed"
	done
}

# require_env <name>...
require_env() {
	local name

	for name in "$@"; do
		[[ -n "${!name:-}" ]] || die "$name is not set"
	done
}

die() {
	printf '%s: %s\n' "$(basename "$0")" "$*" >&2
	exit 1
}

# find_pin <pattern> <file>: echoes what the PCRE pattern matches in the file,
# which may span lines.
find_pin() {
	local -r pattern="$1" file="$2"

	grep -zoP "$pattern" "$file" | tr -d '\0' || die "no pin matching $pattern found in $file"
}

# write_outputs <current> <latest> <pattern>: dies unless latest matches the
# extended regex pattern, then writes `current`, `latest`, and `needs_update`
# to $GITHUB_OUTPUT.
write_outputs() {
	local -r current="$1" latest="$2" pattern="$3"
	local needs_update="true"

	[[ "$latest" =~ $pattern ]] || die "unexpected latest version: $latest"

	if [[ "$current" == "$latest" ]]; then
		needs_update="false"
	fi

	{
		printf 'current=%s\n' "$current"
		printf 'latest=%s\n' "$latest"
		printf 'needs_update=%s\n' "$needs_update"
	} >>"$GITHUB_OUTPUT"

	printf '%s: current %s, latest %s\n' "$(basename "$0")" "$current" "$latest"
}
