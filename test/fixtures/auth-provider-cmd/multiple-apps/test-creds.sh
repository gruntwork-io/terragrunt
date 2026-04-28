#!/usr/bin/env bash

set -euo pipefail

# Use argument if provided, otherwise fallback to PWD
CONFIG_DIR="${1:-${PWD}}"

# shellcheck disable=SC1091  # creds.config is generated at test runtime
. "${CONFIG_DIR}/creds.config"

# Variables (access_key_id, secret_access_key, session_token) are sourced from creds.config
# shellcheck disable=SC2154
{
	if [[ "$access_key_id" != "$AWS_ACCESS_KEY_ID" ]]; then
		exit 1
	fi

	if [[ "$secret_access_key" != "$AWS_SECRET_ACCESS_KEY" ]]; then
		exit 1
	fi

	if [[ "$session_token" != "$AWS_SESSION_TOKEN" ]]; then
		exit 1
	fi
}
