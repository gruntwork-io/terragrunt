#!/usr/bin/env bash

set -euo pipefail

# Hands each unit its own credentials, derived from the unit directory the
# provider runs in, so a run_cmd result leaking across units is observable.
printf '{"envs": {"RUN_CMD_ACCOUNT": "acct-%s"}}\n' "$(basename "$PWD")"
