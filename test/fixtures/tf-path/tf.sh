#!/bin/sh

tfpath="${TG_TF_PATH:-tofu}"

eval "$tfpath $@"
