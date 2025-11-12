#!/bin/bash

# Library script to read release assets configuration
# Usage: source .github/scripts/release/lib-release-config.sh

readonly RELEASE_CONFIG_FILE=".github/assets/release-assets-config.json"

# Get list of all binary filenames
get_all_binaries() {
  jq -r '.platforms[].binary' "$RELEASE_CONFIG_FILE"
}

# Get total binary count (computed from platforms array)
get_binary_count() {
  jq -r '.platforms | length' "$RELEASE_CONFIG_FILE"
}

# Get total expected file count (computed: binaries + archives + additional files)
get_total_file_count() {
  jq -r '
    (.platforms | length) as $binaries |
    (.archive_formats | length) as $formats |
    (.additional_files | length) as $additional |
    $binaries + ($binaries * $formats) + $additional
  ' "$RELEASE_CONFIG_FILE"
}

# Get list of archive extensions
get_archive_extensions() {
  jq -r '.archive_formats[].extension' "$RELEASE_CONFIG_FILE"
}

# Get list of additional files
get_additional_files() {
  jq -r '.additional_files[].name' "$RELEASE_CONFIG_FILE"
}

# Generate expected files list (for verification)
get_all_expected_files() {
  local binaries archive_ext additional

  # Get binaries
  binaries=$(get_all_binaries)

  # Generate list: binaries + archives + additional files
  echo "$binaries"

  # Add archives for each binary
  for binary in $binaries; do
    while IFS= read -r ext; do
      echo "${binary}.${ext}"
    done < <(get_archive_extensions)
  done

  # Add additional files
  get_additional_files
}

# Get platform info as JSON for a specific binary
get_platform_info() {
  local binary="$1"
  jq --arg binary "$binary" '.platforms[] | select(.binary == $binary)' "$RELEASE_CONFIG_FILE"
}

# Generate markdown table rows for summary
generate_platform_table_rows() {
  jq -r '.platforms[] | "| \(.os | ascii_downcase) | \(.arch) | \(if .signed then "Yes" else "No" end) | Uploaded |"' "$RELEASE_CONFIG_FILE" |
  awk '{
    # Capitalize first letter of OS
    if ($2 == "darwin") $2 = "macOS"
    else if ($2 == "linux") $2 = "Linux"
    else if ($2 == "windows") $2 = "Windows"
    print
  }'
}

# Check if config file exists
verify_config_file() {
  if [[ ! -f "$RELEASE_CONFIG_FILE" ]]; then
    echo "ERROR: Release config file not found: $RELEASE_CONFIG_FILE" >&2
    return 1
  fi
}
