#!/bin/bash

set -euo pipefail
echo "$GCLOUD_SERVICE_KEY" > "${HOME}/gcloud-service-key.json"
ls -lahrt ${HOME}/gcloud-service-key.json
file ${HOME}/gcloud-service-key.json
export GOOGLE_APPLICATION_CREDENTIALS="${HOME}/gcloud-service-key.json"

gcloud auth activate-service-account --key-file="${HOME}/gcloud-service-key.json" --quiet
gcloud config set project "${GOOGLE_PROJECT_ID}"
