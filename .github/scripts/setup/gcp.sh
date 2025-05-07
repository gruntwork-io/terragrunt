#!/bin/bash

set -euo pipefail
echo "$GCLOUD_SERVICE_KEY" > "${HOME}/gcloud-service-key.json"
export GOOGLE_APPLICATION_CREDENTIALS="${HOME}/gcloud-service-key.json"
printf "export GOOGLE_APPLICATION_CREDENTIALS='%s'\n" "${HOME}/gcloud-service-key.json" >> "$ENV_FILE"

# Set up gcloud CLI
gcloud auth activate-service-account --key-file="${HOME}/gcloud-service-key.json" --quiet
gcloud config set project "${GOOGLE_PROJECT_ID}"
