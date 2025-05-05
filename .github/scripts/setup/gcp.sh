#!/usr/bin/env bash

set -euo pipefail

# Make GCP Service Account creds available as a file
echo $GCLOUD_SERVICE_KEY > ${HOME}/gcloud-service-key.json
echo 'export GOOGLE_APPLICATION_CREDENTIALS=${HOME}/gcloud-service-key.json' >> $BASH_ENV
# Import test / dev key for SOPS
gpg --import --no-tty --batch --yes ./test/fixtures/sops/test_pgp_key.asc
mkdir -p logs
# configure git to avoid periodic failures
git config --global core.compression 0
git config --global gc.auto 0