#!/usr/bin/env bash

set -euo pipefail
export TOFU_ENGINE_VERSION: "v0.0.16"
export REPO="gruntwork-io/terragrunt-engine-opentofu"
export ASSET_NAME="terragrunt-iac-engine-opentofu_rpc_${TOFU_ENGINE_VERSION}_linux_amd64.zip"
wget -O "engine.zip" "https://github.com/${REPO}/releases/download/${TOFU_ENGINE_VERSION}/${ASSET_NAME}"
unzip -o "engine.zip"
