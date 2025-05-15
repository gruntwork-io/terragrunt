#!/bin/bash

set -euo pipefail

: "${ENV_FILE:?ENV_FILE is not set}"

touch "$ENV_FILE"

printf "export TG_PROVIDER_CACHE='%s'\n" "1" >> "$ENV_FILE"
