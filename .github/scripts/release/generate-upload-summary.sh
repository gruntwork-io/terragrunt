#!/bin/bash

set -euo pipefail

# Script to generate GitHub Actions step summary for release uploads
# Usage: generate-upload-summary.sh
# Environment variables:
#   VERSION: Release version/tag
#   RELEASE_ID: GitHub release ID
#   IS_DRAFT: Whether release was a draft
#   GITHUB_STEP_SUMMARY: Path to GitHub step summary file

# Source configuration library
# shellcheck source=lib-release-config.sh
source "$(dirname "$0")/lib-release-config.sh"

main() {
  require_env_vars VERSION RELEASE_ID IS_DRAFT GITHUB_STEP_SUMMARY
  verify_config_file

  echo "Generating upload summary..."

  local binary_count
  binary_count=$(get_binary_count)
  local total_count
  total_count=$(get_total_file_count)

  cat >>"$GITHUB_STEP_SUMMARY" <<EOF
## Release Asset Upload Summary

**Version**: $VERSION
**Release ID**: $RELEASE_ID
**Was Draft**: $IS_DRAFT

### Assets Uploaded

| Platform | Architecture | Signed | Status |
|----------|--------------|--------|--------|
EOF

  # Generate platform table rows from configuration
  generate_platform_table_rows >>"$GITHUB_STEP_SUMMARY"

  cat >>"$GITHUB_STEP_SUMMARY" <<EOF

**Archive Files**:
- Individual ZIP archives: $binary_count files (one per binary, with +x permissions)
- Individual TAR.GZ archives: $binary_count files (one per binary, with +x permissions)
- **SHA256SUMS**: Checksums for all files

**Total Files**: $total_count ($binary_count binaries + $binary_count ZIPs + $binary_count TAR.GZ + SHA256SUMS)

All assets uploaded successfully to existing release!
EOF

  echo "Upload summary generated successfully"
}

require_env_vars() {
  local missing=0

  for var_name in "$@"; do
    if [[ -z "${!var_name:-}" ]]; then
      echo "ERROR: Required environment variable $var_name not set." >&2
      missing=1
    fi
  done

  if [[ "$missing" -eq 1 ]]; then
    exit 1
  fi
}

main "$@"
