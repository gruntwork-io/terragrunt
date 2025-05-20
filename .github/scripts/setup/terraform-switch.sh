#!/bin/bash

set -euo pipefail

: "${ENV_FILE:?ENV_FILE is not set}"

if [ $# -lt 1 ]; then
  echo "Usage: $0 <terraform-version>"
  exit 1
fi

TF_VERSION="$1"

touch "$ENV_FILE"

mise uninstall opentofu
mise use "terraform@${TF_VERSION}"

terraform --version

printf "export TG_TF_PATH='%s'\n" "terraform" >> "$ENV_FILE"

