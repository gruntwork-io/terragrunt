#!/bin/sh

tfpath="${TG_TF_PATH:-tofu}"

echo "TF script used!" >&2

$tfpath "$@"
