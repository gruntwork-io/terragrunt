#!/usr/bin/env bash
set -euo pipefail

# Run a command without the two side effects a unit test run should not have:
#
# - network reach beyond loopback
# - writes outside the directories the run owns
#
# Written for `go test -exec`:
#
#   go test -exec "$PWD/.github/scripts/ci/sandbox-exec.sh" ./...
#
# Loopback stays reachable so tests can stand up httptest servers, and writes
# stay open in the temp dir, the Go caches and Terragrunt's user cache.
#
# How much of that holds depends on the platform:
#
# - macOS confines both through one seatbelt profile.
# - Linux confines the network alone, through a network namespace.
#
# The --check flag confirms the sandbox is working properly.

CURDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROFILE="$CURDIR/sandbox.sb"

run_sandboxed() {
	case "$(uname -s)" in
	Darwin)
		# sandbox-exec(1) has been deprecated since macOS 10.8, but it is still
		# the only thing on macOS that applies a profile to an arbitrary command,
		# and this never ships anywhere but a contributor's machine. Restrictions
		# reach spawned processes because a child inherits its parent's sandbox,
		# per sandbox(7).
		exec /usr/bin/sandbox-exec \
			-D "TMP=$(cd "${TMPDIR:-/tmp}" && pwd -P)" \
			-D "GOCACHE=$(go env GOCACHE)" \
			-D "GOMODCACHE=$(go env GOMODCACHE)" \
			-D "TGCACHE=$HOME/Library/Caches/terragrunt" \
			-f "$PROFILE" \
			"$@"
		;;
	Linux)
		# A fresh network namespace starts with `lo` down, so bring it up before
		# handing over to the real command. See network_namespaces(7) for what
		# the namespace covers and user_namespaces(7) for what --map-root-user
		# buys an unprivileged caller.
		exec unshare --map-root-user --net -- \
			sh -c 'ip link set lo up && exec "$@"' sh "$@"
		;;
	*)
		echo "sandbox-exec.sh: no sandbox mechanism for $(uname -s)" >&2
		exit 1
		;;
	esac
}

check_egress() {
	local script="$1"

	if ! command -v curl >/dev/null 2>&1; then
		echo "sandbox-exec.sh: --check needs curl" >&2
		exit 1
	fi

	if "$script" curl --silent --show-error --max-time 15 --output /dev/null https://example.com; then
		echo "sandbox-exec.sh: check failed, https://example.com is still reachable inside the sandbox" >&2
		exit 1
	fi

	echo "sandbox-exec.sh: egress is blocked"
}

check_writes() {
	local script="$1"

	local allowed
	allowed="$(mktemp)"

	if ! "$script" touch "$allowed"; then
		rm -f "$allowed"
		echo "sandbox-exec.sh: check failed, the temp dir is not writable inside the sandbox" >&2
		exit 1
	fi

	rm -f "$allowed"

	local refused="$CURDIR/.sandbox-check"

	if "$script" touch "$refused" 2>/dev/null; then
		rm -f "$refused"
		echo "sandbox-exec.sh: check failed, the source tree is still writable inside the sandbox" >&2
		exit 1
	fi

	echo "sandbox-exec.sh: writes are confined to the temp dir and the caches"
}

run_check() {
	local script="${BASH_SOURCE[0]}"

	if ! "$script" true; then
		echo "sandbox-exec.sh: check failed, the sandbox cannot run a command at all" >&2
		exit 1
	fi

	check_egress "$script"

	if [[ "$(uname -s)" != "Darwin" ]]; then
		echo "sandbox-exec.sh: writes are NOT confined on $(uname -s)"
		return
	fi

	check_writes "$script"
}

if [[ "${1:-}" == "--check" ]]; then
	run_check
	exit 0
fi

run_sandboxed "$@"
