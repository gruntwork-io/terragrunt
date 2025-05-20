#!/usr/bin/env bash

set -o pipefail

: "${AWS_TEST_OIDC_ROLE_ARN:?The AWS_TEST_OIDC_ROLE_ARN environment variable must be set.}"
: "${OIDC_TOKEN:?The OIDC_TOKEN environment variable must be set.}"

jq -n \
	--arg role "$AWS_TEST_OIDC_ROLE_ARN" \
	--arg token "$OIDC_TOKEN" \
	'{awsRole: {roleARN: $role, webIdentityToken: $token}}'
