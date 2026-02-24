#!/bin/bash -e

set -o pipefail

# Use argument if provided, otherwise fallback to PWD
CONFIG_DIR="${1:-${PWD}}"

. ${CONFIG_DIR}/creds.config

if [ "$access_key_id" != "$AWS_ACCESS_KEY_ID" ]; then
    exit 1
fi

if [ "$secret_access_key" != "$AWS_SECRET_ACCESS_KEY" ]; then
    exit 1
fi

if [ "$session_token" != "$AWS_SESSION_TOKEN" ]; then
    exit 1
fi
