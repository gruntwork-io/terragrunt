#!/usr/bin/env bash

set -euo pipefail

mise use -g go:golang.org/x/tools/gopls@v0.18.1

gopls version

touch fixed_files.txt
touch gopls_failures.txt
touch gopls_output.txt

while IFS= read -r file; do
    echo "START: $file" | tee -a gopls_output.txt

    if gopls codeaction -kind=quickfix -write "$file"; then
        echo "SUCCESS: $file" | tee -a gopls_output.txt
    else
        echo "FAILED: $file" | tee -a gopls_failures.txt gopls_output.txt
        echo "$file" >> fixed_files.txt
    fi

    echo "END: $file" | tee -a gopls_output.txt
done < gofiles.txt

echo "\n==== gopls failures (if any) ====" | tee -a gopls_output.txt
tee -a gopls_output.txt < gopls_failures.txt || true

# Check if any files were modified
if [ -s fixed_files.txt ]; then
    echo "has_fixes=true" >> "$GITHUB_OUTPUT"
    echo "Files with fixes:" | tee -a gopls_output.txt
    tee -a gopls_output.txt < fixed_files.txt
else
    echo "has_fixes=false" >> "$GITHUB_OUTPUT"
    echo "No files were modified by gopls quickfixes" | tee -a gopls_output.txt
fi
