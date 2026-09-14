#!/usr/bin/env bash

# Returns an awsRole for every invocation, mirroring a real credential broker.
# The role ARN comes from the environment so the test controls it.

set -euo pipefail

printf '{"awsRole": {"roleARN": "%s"}}\n' "${TG_TEST_ROLE_ARN:?TG_TEST_ROLE_ARN must be set}"
