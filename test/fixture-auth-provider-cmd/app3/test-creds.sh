#!/bin/bash -e

. ./creds.config

if [ "$fake_access_key_id" != "$AWS_ACCESS_KEY_ID" ]; then
    exit 1
fi

if [ "$fake_secret_access_key" != "$AWS_SECRET_ACCESS_KEY" ]; then
    exit 1
fi

if [ "$fake_session_token" != "$AWS_SESSION_TOKEN" ]; then
    exit 1
fi
