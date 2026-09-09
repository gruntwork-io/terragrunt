#!/usr/bin/env bash

set -euo pipefail

# Check dependency licenses, keeping the output for the uploaded report.

make license-check | tee license-check.log
