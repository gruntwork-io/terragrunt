#!/usr/bin/env bash

set -euo pipefail

# Required environment variables
: "${NAME:?NAME is not set}"
: "${ENV_FILE:?ENV_FILE is not set}"
: "${GITHUB_WORKSPACE:?GITHUB_WORKSPACE is not set}"
: "${GHA_DEPLOY_KEY:?GHA_DEPLOY_KEY is not set}"

: "${AWS_ACCESS_KEY_ID:?AWS_ACCESS_KEY_ID is not set}"
: "${AWS_SECRET_ACCESS_KEY:?AWS_SECRET_ACCESS_KEY is not set}"
: "${AWS_TEST_S3_ASSUME_ROLE:?AWS_TEST_S3_ASSUME_ROLE is not set}"
: "${AWS_TEST_OIDC_ROLE_ARN:?AWS_TEST_OIDC_ROLE_ARN is not set}"

: "${GCLOUD_SERVICE_KEY:?GCLOUD_SERVICE_KEY is not set}"
: "${GOOGLE_CLOUD_PROJECT:?GOOGLE_CLOUD_PROJECT is not set}"
: "${GOOGLE_COMPUTE_ZONE:?GOOGLE_COMPUTE_ZONE is not set}"
: "${GOOGLE_IDENTITY_EMAIL:?GOOGLE_IDENTITY_EMAIL is not set}"
: "${GOOGLE_PROJECT_ID:?GOOGLE_PROJECT_ID is not set}"
: "${GCLOUD_SERVICE_KEY_IMPERSONATOR:?GCLOUD_SERVICE_KEY_IMPERSONATOR is not set}"

# Optional environment variables
SECRETS="${SECRETS:-}"

touch "$ENV_FILE"

# Manually export each secret listed in matrix.integration.secrets
for SECRET in $SECRETS; do
    if [[ "$SECRET" == "GHA_DEPLOY_KEY" && -n "${GHA_DEPLOY_KEY}" ]]; then
        printf "export GHA_DEPLOY_KEY='%s'\n" "${GHA_DEPLOY_KEY}" >> "$ENV_FILE"
    elif [[ "$SECRET" == "AWS_ACCESS_KEY_ID" && -n "${AWS_ACCESS_KEY_ID}" ]]; then
        printf "export AWS_ACCESS_KEY_ID='%s'\n" "${AWS_ACCESS_KEY_ID}" >> "$ENV_FILE"
    elif [[ "$SECRET" == "AWS_SECRET_ACCESS_KEY" && -n "${AWS_SECRET_ACCESS_KEY}" ]]; then
        printf "export AWS_SECRET_ACCESS_KEY='%s'\n" "${AWS_SECRET_ACCESS_KEY}" >> "$ENV_FILE"
    elif [[ "$SECRET" == "GCLOUD_SERVICE_KEY" && -n "${GCLOUD_SERVICE_KEY}" ]]; then
        printf "export GCLOUD_SERVICE_KEY='%s'\n" "${GCLOUD_SERVICE_KEY}" >> "$ENV_FILE"
        printf "export GOOGLE_SERVICE_ACCOUNT_JSON='%s'\n" "${GCLOUD_SERVICE_KEY}" >> "$ENV_FILE"
    elif [[ "$SECRET" == "GOOGLE_CLOUD_PROJECT" && -n "${GOOGLE_CLOUD_PROJECT}" ]]; then
        printf "export GOOGLE_CLOUD_PROJECT='%s'\n" "${GOOGLE_CLOUD_PROJECT}" >> "$ENV_FILE"
    elif [[ "$SECRET" == "GOOGLE_COMPUTE_ZONE" && -n "${GOOGLE_COMPUTE_ZONE}" ]]; then
        printf "export GOOGLE_COMPUTE_ZONE='%s'\n" "${GOOGLE_COMPUTE_ZONE}" >> "$ENV_FILE"
    elif [[ "$SECRET" == "GOOGLE_IDENTITY_EMAIL" && -n "${GOOGLE_IDENTITY_EMAIL}" ]]; then
        printf "export GOOGLE_IDENTITY_EMAIL='%s'\n" "${GOOGLE_IDENTITY_EMAIL}" >> "$ENV_FILE"
    elif [[ "$SECRET" == "GOOGLE_PROJECT_ID" && -n "${GOOGLE_PROJECT_ID}" ]]; then
        printf "export GOOGLE_PROJECT_ID='%s'\n" "${GOOGLE_PROJECT_ID}" >> "$ENV_FILE"
    elif [[ "$SECRET" == "GCLOUD_SERVICE_KEY_IMPERSONATOR" && -n "${GCLOUD_SERVICE_KEY_IMPERSONATOR}" ]]; then
        printf "export GCLOUD_SERVICE_KEY_IMPERSONATOR='%s'\n" "${GCLOUD_SERVICE_KEY_IMPERSONATOR}" >> "$ENV_FILE"
    elif [[ "$SECRET" == "AWS_ACCESS_KEY_ID" && -n "${AWS_ACCESS_KEY_ID}" ]]; then
        printf "export AWS_ACCESS_KEY_ID='%s'\n" "${AWS_ACCESS_KEY_ID}" >> "$ENV_FILE"
    elif [[ "$SECRET" == "AWS_SECRET_ACCESS_KEY" && -n "${AWS_SECRET_ACCESS_KEY}" ]]; then
        printf "export AWS_SECRET_ACCESS_KEY='%s'\n" "${AWS_SECRET_ACCESS_KEY}" >> "$ENV_FILE"
    elif [[ "$SECRET" == "AWS_TEST_S3_ASSUME_ROLE" && -n "${AWS_TEST_S3_ASSUME_ROLE}" ]]; then
        printf "export AWS_TEST_S3_ASSUME_ROLE='%s'\n" "${AWS_TEST_S3_ASSUME_ROLE}" >> "$ENV_FILE"
    elif [[ "$SECRET" == "AWS_TEST_OIDC_ROLE_ARN" && -n "${AWS_TEST_OIDC_ROLE_ARN}" ]]; then
        printf "export AWS_TEST_OIDC_ROLE_ARN='%s'\n" "${AWS_TEST_OIDC_ROLE_ARN}" >> "$ENV_FILE"
    fi
done

echo "Created environment file with secrets for $NAME"
