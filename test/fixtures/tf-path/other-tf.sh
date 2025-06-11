#!/bin/sh

tfpath="${TG_TF_PATH:-tofu}"

echo "Other TF script used!" >&2

$tfpath "$@"
