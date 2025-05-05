#!/usr/bin/env bash

set -euo pipefail

echo $GCLOUD_SERVICE_KEY > ${HOME}/gcloud-service-key.json
echo 'export GOOGLE_APPLICATION_CREDENTIALS=${HOME}/gcloud-service-key.json' >> $BASH_ENV
