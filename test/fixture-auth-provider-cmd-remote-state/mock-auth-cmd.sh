#!/bin/bash -e

json_string=$( jq -n \
                  --arg access_key_id "__FILL_AWS_ACCESS_KEY_ID__" \
                  --arg secret_access_key "__FILL_AWS_SECRET_ACCESS_KEY__" \
                  --arg session_token "" \
                  --arg tf_var_foo "$tf_var_foo" \
                  '{awsCredentials: {ACCESS_KEY_ID: $access_key_id, SECRET_ACCESS_KEY: $secret_access_key, SESSION_TOKEN: $session_token}, envs: {TF_VAR_foo: $tf_var_foo}}' )
echo $json_string
