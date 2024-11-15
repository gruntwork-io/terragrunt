#!/usr/bin/env bash

set -o pipefail

: "${AWS_TEST_OIDC_ROLE_ARN:?The AWS_TEST_OIDC_ROLE_ARN environment variable must be set.}"
: "${CIRCLE_OIDC_TOKEN_V2:?The CIRCLE_OIDC_TOKEN_V2 environment variable must be set.}"

jq -n \
	--arg role "$AWS_TEST_OIDC_ROLE_ARN" \
	--arg token "$CIRCLE_OIDC_TOKEN_V2" \
	'{awsRole: {roleARN: $role, webIdentityToken: $token}}'
