#!/usr/bin/env bash

set -euo pipefail

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

    # Get file modification time before running gopls
    if [[ -f "$file" ]]; then
        before_mtime=$(stat -c %Y "$file" 2>/dev/null || stat -f %m "$file" 2>/dev/null || echo "0")
    else
        before_mtime="0"
    fi

    if gopls codeaction -kind=quickfix -write -tags="aws,gcp,ssh,sops,tofu,tflint,engine,parse,mocks,private_registry,awsgcp,awsoidc" "$file"; then
        # Check if file was actually modified (has fixes applied)
        if [[ -f "$file" ]]; then
            after_mtime=$(stat -c %Y "$file" 2>/dev/null || stat -f %m "$file" 2>/dev/null || echo "0")
        else
            after_mtime="0"
        fi

        if [[ "$after_mtime" != "$before_mtime" ]]; then
            echo "SUCCESS: $file (fixes applied)" | tee -a "$OUTPUT_FILE"
            echo "$file" >> "$FIXED_FILES"
        else
            echo "SUCCESS: $file (no fixes needed)" | tee -a "$OUTPUT_FILE"
        fi
    else
        echo "FAILED: $file" | tee -a "$FAILURES_FILE" "$OUTPUT_FILE"
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
