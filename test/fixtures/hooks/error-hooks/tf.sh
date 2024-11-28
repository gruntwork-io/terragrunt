#!/bin/sh

(set -x && exec "${TERRAGRUNT_TFPATH:-terraform}" "$@" 2>&1)
