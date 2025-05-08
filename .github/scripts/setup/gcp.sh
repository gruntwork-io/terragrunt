#!/bin/bash

set -euo pipefail

: "${ENV_FILE:?ENV_FILE is not set}"

touch "$ENV_FILE"

echo "$GCLOUD_SERVICE_KEY" > "${HOME}/gcloud-service-key.json"
export GOOGLE_APPLICATION_CREDENTIALS="${HOME}/gcloud-service-key.json"
printf "export GOOGLE_APPLICATION_CREDENTIALS='%s'\n" "${HOME}/gcloud-service-key.json" >> "$ENV_FILE"

# Save gcloud commands to ENV_FILE
printf "gcloud auth activate-service-account --key-file=\"%s\" --quiet\n" "${HOME}/gcloud-service-key.json" >> "$ENV_FILE"
printf "gcloud config set project '%s'\n" "${GOOGLE_PROJECT_ID}" >> "$ENV_FILE"

printf "export GOOGLE_CLOUD_PROJECT='%s'\n" "${GOOGLE_PROJECT_ID}" >> "$ENV_FILE"
