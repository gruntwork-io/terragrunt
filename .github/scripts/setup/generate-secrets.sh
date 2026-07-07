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

# Allowlisted secrets deliverable to a job via matrix.integration.secrets.
declare -A SECRET_VALUES=(
	[GHA_DEPLOY_KEY]="${GHA_DEPLOY_KEY:-}"
	[AWS_ACCESS_KEY_ID]="${AWS_ACCESS_KEY_ID:-}"
	[AWS_SECRET_ACCESS_KEY]="${AWS_SECRET_ACCESS_KEY:-}"
	[AWS_TEST_S3_ASSUME_ROLE]="${AWS_TEST_S3_ASSUME_ROLE:-}"
	[AWS_TEST_OIDC_ROLE_ARN]="${AWS_TEST_OIDC_ROLE_ARN:-}"
	[GCLOUD_SERVICE_KEY]="${GCLOUD_SERVICE_KEY:-}"
	[GOOGLE_CLOUD_PROJECT]="${GOOGLE_CLOUD_PROJECT:-}"
	[GOOGLE_COMPUTE_ZONE]="${GOOGLE_COMPUTE_ZONE:-}"
	[GOOGLE_IDENTITY_EMAIL]="${GOOGLE_IDENTITY_EMAIL:-}"
	[GOOGLE_PROJECT_ID]="${GOOGLE_PROJECT_ID:-}"
	[GCLOUD_SERVICE_KEY_IMPERSONATOR]="${GCLOUD_SERVICE_KEY_IMPERSONATOR:-}"
	[AZURE_CLIENT_ID]="${AZURE_CLIENT_ID:-}"
	[AZURE_CLIENT_SECRET]="${AZURE_CLIENT_SECRET:-}"
	[AZURE_TENANT_ID]="${AZURE_TENANT_ID:-}"
	[TG_AZURE_TEST_STORAGE_ACCOUNT]="${TG_AZURE_TEST_STORAGE_ACCOUNT:-}"
	[TG_AZURE_TEST_SUBSCRIPTION_ID]="${TG_AZURE_TEST_SUBSCRIPTION_ID:-}"
)

# Export each secret listed in matrix.integration.secrets when it has a value.
for SECRET in $SECRETS; do
	value="${SECRET_VALUES[$SECRET]:-}"
	if [[ -z "$value" ]]; then
		continue
	fi

	printf 'export %s=%q\n' "$SECRET" "$value" >>"$ENV_FILE"

	# The GCP service key is also consumed under its legacy alias.
	if [[ "$SECRET" == "GCLOUD_SERVICE_KEY" ]]; then
		printf 'export GOOGLE_SERVICE_ACCOUNT_JSON=%q\n' "$value" >>"$ENV_FILE"
	fi
done

echo "Created environment file with secrets for $NAME"
