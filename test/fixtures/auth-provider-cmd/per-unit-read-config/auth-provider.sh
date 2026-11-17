#!/usr/bin/env bash

# Returns a secret named after the unit directory the command runs in, so every
# unit gets different credentials.

set -euo pipefail

printf '{"envs":{"UNIT_SECRET":"%s"}}\n' "$(basename "$PWD")"
