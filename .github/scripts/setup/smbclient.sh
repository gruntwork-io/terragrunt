#!/usr/bin/env bash

set -euo pipefail

# smbclient is only needed by the docker-tagged SMB tests, and mise has no
# package for it.
sudo apt-get update
sudo apt-get install -y --no-install-recommends smbclient

smbclient --version
