#!/bin/sh

(set -x && exec "${TERRAGRUNT_TFPATH:=tofu}" "$@" 2>&1)
