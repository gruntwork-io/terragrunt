#!/bin/bash

set -euo pipefail
echo "$GCLOUD_SERVICE_KEY" > "${HOME}/gcloud-service-key.json"
export GOOGLE_APPLICATION_CREDENTIALS="${HOME}/gcloud-service-key.json"

gcloud config set project "${GOOGLE_PROJECT_ID}"
