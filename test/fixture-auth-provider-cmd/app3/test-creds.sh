#!/bin/bash -e

. ./creds.config

if [ "$fake_id" != "$AWS_ACCESS_KEY_ID" ]; then
    exit 1
fi

if [ "$fake_key" != "$AWS_SECRET_ACCESS_KEY" ]; then
    exit 1
fi

if [ "$fake_tk" != "$AWS_SESSION_TOKEN" ]; then
    exit 1
fi
