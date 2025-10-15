#!/bin/bash

set -euo pipefail

: "${ENV_FILE:?ENV_FILE is not set}"

touch "$ENV_FILE"

printf "export TG_EXPERIMENT_MODE=%s\n" "true" >> "$ENV_FILE"
