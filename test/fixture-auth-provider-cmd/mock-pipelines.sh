#!/bin/bash -e

. ${PWD}/creds.config

json_string=$( jq -n \
                  --arg access_key_id "$fake_id" \
                  --arg secret_access_key "$fake_key" \
                  --arg session_token "$fake_tk" \
                  '{envs: {AWS_ACCESS_KEY_ID: $access_key_id, AWS_SECRET_ACCESS_KEY: $secret_access_key, AWS_SESSION_TOKEN: $session_token}}' )

echo $json_string
