#!/usr/bin/env bash

set -euo pipefail

# Required environment variables
: "${ENV_FILE:?ENV_FILE is not set}"

# Optional environment variables
SETUP_SCRIPTS="${SETUP_SCRIPTS:-}"

# Source the environment file
# shellcheck source=/dev/null
source "${ENV_FILE}"

# Loop through setup scripts and execute them
for SCRIPT in $SETUP_SCRIPTS; do
    echo "Running setup script: $SCRIPT"
    "$SCRIPT"
    echo "Setup script $SCRIPT completed"
done
