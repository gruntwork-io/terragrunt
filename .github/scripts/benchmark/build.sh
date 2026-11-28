#!/usr/bin/env bash

set -euo pipefail

# Build everything a benchmark run needs into <work-dir>, from the checkout this
# script runs in (head) and from <base-ref>.
#
# Usage: build.sh <base-ref> <work-dir>
#
# Layout written:
#   bin/head/terragrunt     Terragrunt built from this checkout.
#   bin/base/terragrunt     Terragrunt built from <base-ref>.
#   bin/tools/benchfixture  Fixture generator, from this checkout.
#   bin/fake/tofu           faketofu, from this checkout.
#   src/base                Detached worktree of <base-ref>.
#
# Both sides run against this checkout's fixtures and faketofu, so a change to
# either affects both builds equally.

function main {
	local -r base_ref="${1:?Usage: build.sh <base-ref> <work-dir>}"
	local -r work_dir="${2:?Usage: build.sh <base-ref> <work-dir>}"

	mkdir -p "${work_dir}/bin/base" "${work_dir}/bin/head" "${work_dir}/bin/tools" "${work_dir}/bin/fake" "${work_dir}/src"

	git worktree add --detach "${work_dir}/src/base" "$base_ref"

	build_terragrunt . "${work_dir}/bin/head/terragrunt"
	build_terragrunt "${work_dir}/src/base" "${work_dir}/bin/base/terragrunt"

	CGO_ENABLED=0 go build -o "${work_dir}/bin/tools/benchfixture" ./test/benchmarks/cmd/benchfixture
	CGO_ENABLED=0 go build -o "${work_dir}/bin/fake/tofu" ./test/benchmarks/cmd/faketofu

	return 0
}

function build_terragrunt {
	local -r source_dir="$1"
	local -r output="$2"

	CGO_ENABLED=0 go -C "$source_dir" build -trimpath -o "$output" .

	return 0
}

main "$@"
