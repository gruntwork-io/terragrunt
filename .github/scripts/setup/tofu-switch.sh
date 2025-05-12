#!/bin/bash

set -euo pipefail

: "${ENV_FILE:?ENV_FILE is not set}"

touch "$ENV_FILE"

mise uninstall --all terraform

tofu --version

printf "export TG_TF_PATH='%s'\n" "tofu" >> "$ENV_FILE"

