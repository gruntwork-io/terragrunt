#!/usr/bin/env bash

set -euo pipefail

: "${ENV_FILE:?ENV_FILE is not set}"
: "${SETUP_SCRIPTS:?SETUP_SCRIPTS is not set}"

# Source the environment file
# shellcheck source=/dev/null
source "${ENV_FILE}"

# Loop through setup scripts and execute them
for SCRIPT in $SETUP_SCRIPTS; do
    if [[ -n "$SCRIPT" ]]; then
        echo "Running setup script: $SCRIPT"
        "$SCRIPT"
    fi
done
