#!/usr/bin/env bash
set -euo pipefail

# Run a command with no network reach beyond loopback.
#
# Written for `go test -exec`, which splits the flag value on spaces, so this
# has to be a single path taking the test binary and its arguments:
#
#   go test -exec "$PWD/.github/scripts/ci/no-network-exec.sh" ./...
#
# Loopback stays reachable so tests can stand up httptest servers. Everything
# else is refused, which turns any unvirtualized network call in the unit suite
# into a test failure.
#
# With --check, confirm the sandbox is actually in force.

PROFILE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/no-network.sb"

run_sandboxed() {
	case "$(uname -s)" in
	Darwin)
		exec sandbox-exec -f "$PROFILE" "$@"
		;;
	Linux)
		# A fresh network namespace starts with `lo` down, so bring it up before
		# handing over to the real command.
		exec unshare --map-root-user --net -- \
			sh -c 'ip link set lo up && exec "$@"' sh "$@"
		;;
	*)
		echo "no-network-exec.sh: no sandbox mechanism for $(uname -s)" >&2
		exit 1
		;;
	esac
}

run_check() {
	local script="${BASH_SOURCE[0]}"

	if ! command -v curl >/dev/null 2>&1; then
		echo "no-network-exec.sh: --check needs curl" >&2
		exit 1
	fi

	# A sandbox that cannot start makes every command under it fail, which would
	# otherwise read as "egress is blocked" below and hand back a green run.
	if ! "$script" true; then
		echo "no-network-exec.sh: check failed, the sandbox cannot run a command at all" >&2
		exit 1
	fi

	if "$script" curl --silent --show-error --max-time 15 --output /dev/null https://example.com; then
		echo "no-network-exec.sh: check failed, https://example.com is still reachable inside the sandbox" >&2
		exit 1
	fi

	echo "no-network-exec.sh: check passed, egress is blocked"
}

if [[ "${1:-}" == "--check" ]]; then
	run_check
	exit 0
fi

run_sandboxed "$@"
