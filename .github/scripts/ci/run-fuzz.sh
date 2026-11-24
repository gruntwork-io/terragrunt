#!/usr/bin/env bash

set -euo pipefail

# Run every fuzz test in the module for FUZZ_TIME each. A failing fuzz test
# does not stop the run; the script exits non-zero after all of them finish.

FUZZ_TIME="${FUZZ_TIME:-30s}"

packages=$(go list ./...)
failed=()

for package in $packages; do
	listed=$(go test -list '^Fuzz' "$package")

	for fuzz_test in $(grep '^Fuzz' <<<"$listed" || true); do
		echo "Fuzzing $fuzz_test in $package"

		if ! gotestsum --format testname -- -run '^$' -fuzztime="$FUZZ_TIME" -fuzz "^${fuzz_test}\$" "$package"; then
			failed+=("$package.$fuzz_test")
		fi
	done
done

if ((${#failed[@]} > 0)); then
	echo "Fuzz tests failed:" >&2
	printf '  %s\n' "${failed[@]}" >&2
	exit 1
fi
