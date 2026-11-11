#!/usr/bin/env bash

set -euo pipefail

main() {
	: "${ENV_FILE:?ENV_FILE is not set}"

	touch "$ENV_FILE"

	printf "export TG_TMP_CONFIG_REWRITE=%s\n" "true" >>"$ENV_FILE"
}

main "$@"
