#!/bin/sh

(set -x && exec "${TG_TF_PATH:-tofu}" "$@" 2>&1)
