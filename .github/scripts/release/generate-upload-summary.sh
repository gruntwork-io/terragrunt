#!/bin/bash

set -e

# Script to generate GitHub Actions step summary for release uploads
# Usage: generate-upload-summary.sh
# Environment variables:
#   VERSION: Release version/tag
#   RELEASE_ID: GitHub release ID
#   IS_DRAFT: Whether release was a draft
#   GITHUB_STEP_SUMMARY: Path to GitHub step summary file

function main {
  assert_env_var_not_empty "VERSION"
  assert_env_var_not_empty "RELEASE_ID"
  assert_env_var_not_empty "IS_DRAFT"
  assert_env_var_not_empty "GITHUB_STEP_SUMMARY"

  echo "Generating upload summary..."

  # Header
  echo "## Release Asset Upload Summary" >> "$GITHUB_STEP_SUMMARY"
  echo "" >> "$GITHUB_STEP_SUMMARY"

  # Release details
  printf '**Version**: %s\n' "$VERSION" >> "$GITHUB_STEP_SUMMARY"
  printf '**Release ID**: %s\n' "$RELEASE_ID" >> "$GITHUB_STEP_SUMMARY"
  printf '**Was Draft**: %s\n' "$IS_DRAFT" >> "$GITHUB_STEP_SUMMARY"
  echo "" >> "$GITHUB_STEP_SUMMARY"

  # Assets uploaded section
  echo "### Assets Uploaded" >> "$GITHUB_STEP_SUMMARY"
  echo "" >> "$GITHUB_STEP_SUMMARY"

  # Platform table
  echo "| Platform | Architecture | Signed | Status |" >> "$GITHUB_STEP_SUMMARY"
  echo "|----------|--------------|--------|--------|" >> "$GITHUB_STEP_SUMMARY"
  echo "| macOS    | amd64        | Yes    | Uploaded |" >> "$GITHUB_STEP_SUMMARY"
  echo "| macOS    | arm64        | Yes    | Uploaded |" >> "$GITHUB_STEP_SUMMARY"
  echo "| Linux    | 386          | No     | Uploaded |" >> "$GITHUB_STEP_SUMMARY"
  echo "| Linux    | amd64        | No     | Uploaded |" >> "$GITHUB_STEP_SUMMARY"
  echo "| Linux    | arm64        | No     | Uploaded |" >> "$GITHUB_STEP_SUMMARY"
  echo "| Windows  | 386          | Yes    | Uploaded |" >> "$GITHUB_STEP_SUMMARY"
  echo "| Windows  | amd64        | Yes    | Uploaded |" >> "$GITHUB_STEP_SUMMARY"
  echo "" >> "$GITHUB_STEP_SUMMARY"

  # Archive files section
  echo "**Archive Files**:" >> "$GITHUB_STEP_SUMMARY"
  echo "- Individual ZIP archives: 7 files (one per binary, with +x permissions)" >> "$GITHUB_STEP_SUMMARY"
  echo "- Individual TAR.GZ archives: 7 files (one per binary, with +x permissions)" >> "$GITHUB_STEP_SUMMARY"
  echo "- **SHA256SUMS**: Checksums for all files" >> "$GITHUB_STEP_SUMMARY"
  echo "" >> "$GITHUB_STEP_SUMMARY"

  # Total files
  echo "**Total Files**: 22 (7 binaries + 7 ZIPs + 7 TAR.GZ + SHA256SUMS)" >> "$GITHUB_STEP_SUMMARY"
  echo "" >> "$GITHUB_STEP_SUMMARY"

  # Success message
  echo "All assets uploaded successfully to existing release!" >> "$GITHUB_STEP_SUMMARY"

  echo "Upload summary generated successfully"
}

function assert_env_var_not_empty {
  local -r var_name="$1"
  local -r var_value="${!var_name}"

  if [[ -z "$var_value" ]]; then
    echo "ERROR: Required environment variable $var_name not set."
    exit 1
  fi
}

main "$@"
