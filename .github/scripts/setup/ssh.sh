#!/usr/bin/env bash

set -euo pipefail

SSH_KEY="${SSH_KEY:?Required environment variable SSH_KEY}"

mkdir -p ~/.ssh
echo "$SSH_KEY" > ~/.ssh/id_rsa
chmod 600 ~/.ssh/id_rsa
