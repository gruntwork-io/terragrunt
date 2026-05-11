#!/usr/bin/env bash

set -euo pipefail

gpg --import --no-tty --batch --yes ./test/fixtures/sops/test_pgp_key.asc

# Export to legacy pubring.gpg/secring.gpg for SOPS v3.13's Go PGP backend.
gpg --export >"$HOME/.gnupg/pubring.gpg"
gpg --export-secret-keys >"$HOME/.gnupg/secring.gpg"
chmod 600 "$HOME/.gnupg/secring.gpg"
