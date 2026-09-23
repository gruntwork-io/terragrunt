#!/usr/bin/env bash

set -euo pipefail

# Cross-compile a terragrunt binary into bin/ for every published platform.

: "${BUILD_VERSION:?BUILD_VERSION is not set}"

# shellcheck source=../release/lib-release-config.sh
source .github/scripts/release/lib-release-config.sh

verify_config_file

while read -r target_os target_arch binary; do
	echo "::group::Building ${target_os}/${target_arch}"

	CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -o "bin/${binary}" \
		-ldflags "-s -w -X github.com/gruntwork-io/terragrunt/internal/version.Version=${BUILD_VERSION}" \
		.

	.github/scripts/release/verify-static-binary.sh "bin/${binary}" "$target_os" "$target_arch"

	echo "::endgroup::"
done < <(get_platform_targets)
