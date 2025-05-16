#!/bin/bash
# Download custom build of Tofu
set -euo pipefail

: "${ENV_FILE:?ENV_FILE is not set}"

touch "$ENV_FILE"

mise uninstall --all terraform
mise uninstall --all opentofu

# Download custom build of Tofu from https://github.com/denis256/opentofu/releases/download/experiment_prune_provider_schemas-1/tofu_linux_amd64

wget https://github.com/denis256/opentofu/releases/download/experiment_prune_provider_schemas-1/tofu_linux_amd64
chmod +x tofu_linux_amd64
# put it in path under tofu_linux_amd64
mv tofu_linux_amd64 /usr/local/bin/tofu

tofu --version

printf "export TG_TF_PATH='%s'\n" "tofu" >> "$ENV_FILE"

