#!/bin/bash

set -euo pipefail

: "${ENV_FILE:?ENV_FILE is not set}"

touch "$ENV_FILE"

printf "export TG_EXPERIMENT='%s'\n" "cas" >> "$ENV_FILE"
