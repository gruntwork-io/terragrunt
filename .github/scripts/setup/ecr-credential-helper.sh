#!/usr/bin/env bash

set -euo pipefail

: "${ENV_FILE:?ENV_FILE is not set}"

VERSION="0.10.1"
SHA256="ae5f2c0f1a2283f687e24a529f41d58f29c0a5bcfa0b60d1d3f8dc33b7eac4f2"
URL="https://amazon-ecr-credential-helper-releases.s3.us-east-2.amazonaws.com/${VERSION}/linux-amd64/docker-credential-ecr-login"

DEST_DIR="${RUNNER_TEMP:-/tmp}/ecr-credential-helper"
mkdir -p "$DEST_DIR"

curl -fsSL --proto '=https' --tlsv1.2 -o "$DEST_DIR/docker-credential-ecr-login" "$URL"
echo "${SHA256}  ${DEST_DIR}/docker-credential-ecr-login" | sha256sum -c -
chmod +x "$DEST_DIR/docker-credential-ecr-login"

touch "$ENV_FILE"
printf "export PATH='%s:'\"\$PATH\"\n" "$DEST_DIR" >>"$ENV_FILE"

echo "Installed docker-credential-ecr-login ${VERSION} to ${DEST_DIR}"
