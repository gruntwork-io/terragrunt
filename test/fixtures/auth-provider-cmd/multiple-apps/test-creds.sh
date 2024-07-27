#!/bin/bash -e

set -o pipefail

. ${PWD}/creds.config

if [ "$access_key_id" != "$AWS_ACCESS_KEY_ID" ]; then
    exit 1
fi

if [ "$secret_access_key" != "$AWS_SECRET_ACCESS_KEY" ]; then
    exit 1
fi

if [ "$session_token" != "$AWS_SESSION_TOKEN" ]; then
    exit 1
fi
