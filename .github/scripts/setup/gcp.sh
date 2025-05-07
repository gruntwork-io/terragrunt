#!/bin/bash

set -euo pipefail

: "${ENV_FILE:?ENV_FILE is not set}"

touch "$ENV_FILE"

echo "$GCLOUD_SERVICE_KEY" > "${HOME}/gcloud-service-key.json"
export GOOGLE_APPLICATION_CREDENTIALS="${HOME}/gcloud-service-key.json"
printf "export GOOGLE_APPLICATION_CREDENTIALS='%s'\n" "${HOME}/gcloud-service-key.json" >> "$ENV_FILE"
printf "export GOOGLE_PROJECT_ID='%s'\n" "terragrunt-458620" >> "$ENV_FILE"

# Set up gcloud CLI
gcloud auth activate-service-account --key-file="${HOME}/gcloud-service-key.json" --quiet
gcloud config set project "${GOOGLE_PROJECT_ID}"
