#!/usr/bin/env bash
# Fake OpenTofu binary: answers the version probe and prints the cache env var for any other command.
if [ "$1" = "-version" ]; then
	echo "OpenTofu v1.10.0"
	exit 0
fi
echo "FAKE_TOFU TF_PLUGIN_CACHE_DIR=${TF_PLUGIN_CACHE_DIR}"
