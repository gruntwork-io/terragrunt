#!/usr/bin/env bash

set -euo pipefail

# Required environment variables
: "${NAME:?NAME is not set}"
: "${ENV_FILE:?ENV_FILE is not set}"
: "${GITHUB_WORKSPACE:?GITHUB_WORKSPACE is not set}"
: "${GHA_DEPLOY_KEY:?GHA_DEPLOY_KEY is not set}"

# Optional environment variables
SECRETS="${SECRETS:-}"

touch "$ENV_FILE"

# Values are %q-quoted so shell metacharacters cannot terminate the assignment.
# Manually export each secret listed in matrix.integration.secrets
for SECRET in $SECRETS; do
	if [[ -z "${!SECRET:-}" ]]; then
		echo "$SECRET is not set" >&2
		exit 1
	fi

	case "$SECRET" in
	"GCLOUD_SERVICE_KEY")
		printf "export GCLOUD_SERVICE_KEY=%q\n" "${GCLOUD_SERVICE_KEY}" >>"$ENV_FILE"
		printf "export GOOGLE_SERVICE_ACCOUNT_JSON=%q\n" "${GCLOUD_SERVICE_KEY}" >>"$ENV_FILE"
		;;
	"GHA_DEPLOY_KEY" | "AWS_ACCESS_KEY_ID" | "AWS_SECRET_ACCESS_KEY" | "AWS_TEST_S3_ASSUME_ROLE" | \
		"AWS_TEST_OIDC_ROLE_ARN" | "AWS_TEST_OIDC_CHAIN_SOURCE_ROLE_ARN" | "AWS_TEST_OIDC_CHAIN_TARGET_ROLE_ARN" | \
		"GOOGLE_CLOUD_PROJECT" | "GOOGLE_COMPUTE_ZONE" | "GOOGLE_IDENTITY_EMAIL" | "GOOGLE_PROJECT_ID" | \
		"GCLOUD_SERVICE_KEY_IMPERSONATOR" | "AZURE_CLIENT_ID" | "AZURE_CLIENT_SECRET" | "AZURE_TENANT_ID" | \
		"TG_AZURE_TEST_STORAGE_ACCOUNT" | "TG_AZURE_TEST_SUBSCRIPTION_ID" | "TG_AZURE_TEST_RESOURCE_GROUP" | \
		"ARM_CLIENT_ID" | "ARM_CLIENT_SECRET" | "ARM_TENANT_ID" | "ARM_SUBSCRIPTION_ID")
		printf "export %s=%q\n" "$SECRET" "${!SECRET}" >>"$ENV_FILE"
		;;
	*)
		echo "$SECRET is not supported" >&2
		exit 1
		;;
	esac
done

echo "Created environment file with secrets for $NAME"
