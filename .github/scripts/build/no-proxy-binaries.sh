#!/usr/bin/env bash

set -euo pipefail

# Cross-compile every published platform with the Go module proxy disabled,
# proving the module graph still resolves straight from its source repositories.
# The binaries are never published, so only the compile has to succeed.

: "${GITHUB_REF_NAME:?GITHUB_REF_NAME is not set}"

# shellcheck source=../release/lib-release-config.sh
source .github/scripts/release/lib-release-config.sh

verify_config_file

export GOPROXY=direct

# git-lfs ships on ubuntu runners and would smudge LFS pointers into full files
# while fetching modules from VCS, producing a zip hash go.sum does not record.
export GIT_LFS_SKIP_SMUDGE=1

while read -r target_os target_arch binary; do
	echo "::group::Building ${target_os}/${target_arch}"

	GOOS="$target_os" GOARCH="$target_arch" go build -o "bin/${binary}" \
		-ldflags "-X github.com/gruntwork-io/terragrunt/internal/version.Version=${GITHUB_REF_NAME} -extldflags '-static'" \
		.

	echo "::endgroup::"
done < <(get_platform_targets)
