#!/usr/bin/env bash
# Fake Terraform binary: answers the version probe and prints the cache env var for any other command.
if [ "$1" = "-version" ]; then
	echo "Terraform v1.5.7"
	exit 0
fi
echo "FAKE_TERRAFORM TF_PLUGIN_CACHE_DIR=${TF_PLUGIN_CACHE_DIR}"
