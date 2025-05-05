#!/usr/bin/env bash

set -euo pipefail

SSH_KEY="${GHA_DEPLOY_KEY:?Required environment variable GHA_DEPLOY_KEY}"

mkdir -p ~/.ssh
echo "$SSH_KEY" > ~/.ssh/id_rsa
chmod 600 ~/.ssh/id_rsa
