#!/usr/bin/env bash

set -euo pipefail

# gopls is installed by the mise-action step from mise.toml; do not pin a
# separate version here (older releases fail to build under newer Go).
gopls version

# gopls executes only the first matching code action per invocation, so each
# file is processed repeatedly until no quickfix remains. This bounds the loop
# in case an action is ever non-converging.
MAX_PASSES=10

TEMP_DIR=$(mktemp -d)
FIXED_FILES="$TEMP_DIR/fixed_files.txt"
FAILURES_FILE="$TEMP_DIR/gopls_failures.txt"
OUTPUT_FILE="$TEMP_DIR/gopls_output.txt"

touch "$FIXED_FILES"
touch "$FAILURES_FILE"
touch "$OUTPUT_FILE"

while IFS= read -r file; do
	echo "START: $file" | tee -a "$OUTPUT_FILE"

	for ((pass = 1; pass <= MAX_PASSES; pass++)); do
		if output=$(gopls codeaction -kind=quickfix -exec -w "$file" 2>&1); then
			# An action was applied; record it and look for more.
			printf 'APPLIED (pass %d): %s\n%s\n' "$pass" "$file" "$output" | tee -a "$OUTPUT_FILE"
			continue
		fi

		# Non-zero exit. "no matching code action" is the normal "nothing (more)
		# to fix" case (gopls exits 2); anything else is a real failure.
		if grep -q 'no matching code action' <<<"$output"; then
			echo "NO FIX: $file" | tee -a "$OUTPUT_FILE"
		else
			printf 'FAILED: %s\n%s\n' "$file" "$output" | tee -a "$FAILURES_FILE" "$OUTPUT_FILE"
		fi
		break
	done

	echo "END: $file" | tee -a "$OUTPUT_FILE"
done <"${GOFILES_LIST:-gofiles.txt}"

printf '\n==== gopls failures (if any) ====\n' | tee -a "$OUTPUT_FILE"
tee -a "$OUTPUT_FILE" <"$FAILURES_FILE" || true

# The set of fixed files is whatever gopls actually changed in the working tree,
# not what it logged. This is the source of truth for whether a PR is needed.
git diff --name-only >"$FIXED_FILES"

if [[ -s "$FIXED_FILES" ]]; then
	echo "has_fixes=true" >>"$GITHUB_OUTPUT"
	echo "Files with fixes:" | tee -a "$OUTPUT_FILE"
	tee -a "$OUTPUT_FILE" <"$FIXED_FILES"
else
	echo "has_fixes=false" >>"$GITHUB_OUTPUT"
	echo "No files were modified by gopls quickfixes" | tee -a "$OUTPUT_FILE"
fi

# Output file paths for other steps to use.
echo "fixed_files_path=$FIXED_FILES" >>"$GITHUB_OUTPUT"
echo "output_file_path=$OUTPUT_FILE" >>"$GITHUB_OUTPUT"
