#!/usr/bin/env bash

set -euo pipefail

# Fail unless every macOS binary the release config lists came back from signing.

# shellcheck source=lib-release-config.sh
source .github/scripts/release/lib-release-config.sh

verify_config_file

while read -r binary; do
	echo "Checking bin/${binary}:"

	if [[ ! -f "bin/${binary}" ]]; then
		echo "  ERROR: Binary bin/${binary} is missing from downloaded artifact"
		exit 1
	fi

	echo "  OK: bin/${binary}"
	file "bin/${binary}"

	# The merge step creates any zip that signing did not, so a missing one here
	# is not yet a failure.
	if [[ -f "bin/${binary}.zip" ]]; then
		echo "  OK: bin/${binary}.zip"
	else
		echo "  NOTICE: bin/${binary}.zip (not present, will be created in merge)"
	fi

	echo
done < <(get_binaries_for_os darwin)
