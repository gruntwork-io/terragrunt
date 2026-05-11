#!/usr/bin/env bash

set -euo pipefail

gpg --import --no-tty --batch --yes ./test/fixtures/sops/test_pgp_key.asc
