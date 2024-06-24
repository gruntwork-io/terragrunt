#!/bin/bash
set -e
set -u
set -o pipefail

# Script to verify the release assets by downloading them from the GitHub release
# and checking if they are accessible. If the asset is not accessible, the script
# will try to delete asset and re-upload it to the release.
# https://github.com/gruntwork-io/terragrunt/issues/3220

# Ensure CIRCLE_TAG and GITHUB_OAUTH_TOKEN are set
if [ -z "${CIRCLE_TAG:-}" ]; then
  echo "CIRCLE_TAG environment variable is not set. Exiting..."
  exit 1
fi

if [ -z "${GITHUB_OAUTH_TOKEN:-}" ]; then
  echo "GITHUB_OAUTH_TOKEN environment variable is not set. Exiting..."
  exit 1
fi

RELEASE_TAG=$CIRCLE_TAG
REPO_OWNER="gruntwork-io"
REPO_NAME="terragrunt"
MAX_RETRIES=10

RELEASE_RESPONSE=$(curl -s \
  -H "Accept: application/vnd.github.v3+json" \
  -H "Authorization: token $GITHUB_OAUTH_TOKEN" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  "https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/releases/tags/$RELEASE_TAG")

# Check if the release exists
if jq -e '.message == "Not Found"' <<< "$RELEASE_RESPONSE" > /dev/null; then
  echo "Release $RELEASE_TAG not found. Exiting..."
  exit 1
fi

# Get the release id
RELEASE_ID=$(echo "$RELEASE_RESPONSE" | jq -r '.id')
ASSET_URLS=$(echo "$RELEASE_RESPONSE" | jq -r '.assets[].browser_download_url')

# Loop through each asset URL and attempt to download
for ASSET_URL in $ASSET_URLS; do
  ASSET_NAME=$(basename "$ASSET_URL")

  for ((i=0; i<MAX_RETRIES; i++)); do
    if ! curl -sILf "$ASSET_URL" > /dev/null; then
      echo "Failed to download the asset $ASSET_NAME. Retrying..."

      # Delete the asset
      ASSET_ID=$(jq -r --arg asset_name "$ASSET_NAME" '.assets[] | select(.name == $asset_name) | .id' <<< "$RELEASE_RESPONSE")
      curl -s -L -XDELETE \
        -H "Accept: application/vnd.github.v3+json" \
        -H "Authorization: token $GITHUB_OAUTH_TOKEN" \
        -H "X-GitHub-Api-Version: 2022-11-28" \
        "https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/releases/assets/$ASSET_ID" > /dev/null

      # Re-upload the asset
      curl -s -L -XPOST \
        -H "Accept: application/vnd.github.v3+json" \
        -H "Authorization: token $GITHUB_OAUTH_TOKEN" \
        -H "X-GitHub-Api-Version: 2022-11-28" \
        -H "Content-Type: application/octet-stream" \
        --data-binary "@bin/$ASSET_NAME" \
        "https://uploads.github.com/repos/$REPO_OWNER/$REPO_NAME/releases/$RELEASE_ID/assets?name=$ASSET_NAME" > /dev/null
    else
      echo "Successfully checked the asset $ASSET_NAME"
      break
    fi
  done

  if (( i == MAX_RETRIES )); then
    echo "Failed to download the asset $ASSET_NAME after $MAX_RETRIES retries. Exiting..."
    exit 1
  fi
done

echo "All assets checks passed."
