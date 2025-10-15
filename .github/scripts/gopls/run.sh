#!/usr/bin/env bash

set -euo pipefail

mise use -g go:golang.org/x/tools/gopls@v0.18.1

gopls version

TEMP_DIR=$(mktemp -d)
FIXED_FILES="$TEMP_DIR/fixed_files.txt"
FAILURES_FILE="$TEMP_DIR/gopls_failures.txt"
OUTPUT_FILE="$TEMP_DIR/gopls_output.txt"

touch "$FIXED_FILES"
touch "$FAILURES_FILE"
touch "$OUTPUT_FILE"

while IFS= read -r file; do
    echo "START: $file" | tee -a "$OUTPUT_FILE"

    if gopls codeaction -kind=quickfix -write "$file"; then
        echo "SUCCESS: $file" | tee -a "$OUTPUT_FILE"
    else
        echo "FAILED: $file" | tee -a "$FAILURES_FILE" "$OUTPUT_FILE"
        echo "$file" >> "$FIXED_FILES"
    fi

    echo "END: $file" | tee -a "$OUTPUT_FILE"
done < gofiles.txt

echo "\n==== gopls failures (if any) ====" | tee -a "$OUTPUT_FILE"
tee -a "$OUTPUT_FILE" < "$FAILURES_FILE" || true

# Check if any files were modified
if [ -s "$FIXED_FILES" ]; then
    echo "has_fixes=true" >> "$GITHUB_OUTPUT"
    echo "Files with fixes:" | tee -a "$OUTPUT_FILE"
    tee -a "$OUTPUT_FILE" < "$FIXED_FILES"
else
    echo "has_fixes=false" >> "$GITHUB_OUTPUT"
    echo "No files were modified by gopls quickfixes" | tee -a "$OUTPUT_FILE"
fi

# Output file paths for other steps to use
echo "fixed_files_path=$FIXED_FILES" >> "$GITHUB_OUTPUT"
echo "output_file_path=$OUTPUT_FILE" >> "$GITHUB_OUTPUT"
