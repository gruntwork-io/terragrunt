#!/usr/bin/env bash

set -euo pipefail

# tflint is only needed by the tflint integration tests, so it is installed here
# rather than in mise.toml to keep local installs lean.
mise use "tflint@0.50.3"

tflint --version
