#!/bin/bash -e

set -o pipefail

. ${PWD}/creds.config

json_string=$(jq -n \
	--arg access_key_id "$access_key_id" \
	--arg secret_access_key "$secret_access_key" \
	--arg session_token "$session_token" \
	--arg tf_var_foo "$tf_var_foo" \
	'{awsCredentials: {ACCESS_KEY_ID: $access_key_id, SECRET_ACCESS_KEY: $secret_access_key, SESSION_TOKEN: $session_token}, envs: {TF_VAR_foo: $tf_var_foo}}')

echo $json_string
