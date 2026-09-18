#!/usr/bin/env bash
#
# Runs FuzzFullCLI without end. Each failure is filed under its signature in
# the findings directory, its signature is added to the skip list the test
# reads, and fuzzing restarts.
#
# Usage: full-cli.sh [findings-dir]
#
# Environment:
#   FUZZ_CHUNK           how long one fuzz run lasts before restarting (default 1h)
#   FUZZ_PARALLEL        fuzz workers (default: number of CPUs)
#   FUZZ_KEEP_INPUTS     failing inputs kept per signature (default 5)
#   FUZZ_MAX_ERRORS      consecutive runs that fail without an input before
#                        giving up, e.g. on a build error (default 3)
#
# Findings layout:
#   skip-list.txt              one signature per line, read by the test
#   <id>/signature             the failure's signature
#   <id>/count                 how many times it was hit
#   <id>/inputs/<name>         failing inputs; replay one by copying it into
#                              test/testdata/fuzz/FuzzFullCLI/ and running
#                              go test -run 'FuzzFullCLI/<name>' ./test/
#   <id>/<timestamp>.log       go test output of each hit with a kept input
#   errors/<timestamp>.log     output of runs that failed without an input

set -euo pipefail

REPO="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
readonly REPO
FINDINGS="${1:-$HOME/fuzz-findings/full-cli}"
mkdir -p "$FINDINGS"
FINDINGS="$(cd "$FINDINGS" && pwd)"
readonly FINDINGS
readonly CORPUS="$REPO/test/testdata/fuzz/FuzzFullCLI"
readonly SKIP_LIST="$FINDINGS/skip-list.txt"
readonly CHUNK="${FUZZ_CHUNK:-1h}"
readonly PARALLEL="${FUZZ_PARALLEL:-$(getconf _NPROCESSORS_ONLN)}"
readonly KEEP_INPUTS="${FUZZ_KEEP_INPUTS:-5}"
readonly MAX_ERRORS="${FUZZ_MAX_ERRORS:-3}"

# The test reads this; see EnvFuzzSkipList in test/fuzz_cli_test.go.
export TG_TEST_FUZZ_SKIP_LIST="$SKIP_LIST"

go_cmd() {
	if command -v mise >/dev/null; then
		mise x go -- go "$@"
	else
		go "$@"
	fi
}

log() {
	printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >&2
}

list_corpus() {
	mkdir -p "$CORPUS"
	find "$CORPUS" -maxdepth 1 -type f -exec basename {} \; | sort
}

# crash_signature derives a signature from the output of a worker that died
# on a panic the test could not catch. It must match fuzzPanicSignature in
# test/fuzz_cli_test.go: the first line of the panic message with digits
# replaced by N and everything from the first quote or parenthesis dropped,
# then the first Terragrunt function after the last panic( frame.
crash_signature() {
	awk '
		{ sub(/^[ \t]+/, "") }
		!msg && /^panic: / {
			msg = $0
			sub(/^panic: /, "", msg)
			sub(/ \[recovered\].*$/, "", msg)
			collecting = 1
			next
		}
		collecting && (/^created by / || /^exit status/ || /^FAIL/) { collecting = 0 }
		collecting { lines[n++] = $0 }
		END {
			if (!msg) exit
			start = 0
			for (i = 0; i < n; i++) if (lines[i] ~ /^panic\(/) start = i + 1
			frame = "unknown"
			for (i = start; i < n; i++) {
				if (index(lines[i], "github.com/gruntwork-io/terragrunt/") == 1) {
					frame = lines[i]
					sub(/\([^()]*\)$/, "", frame)
					break
				}
			}
			gsub(/[0-9]+/, "N", msg)
			sub(/[("\047].*$/, "", msg)
			gsub(/^ +| +$/, "", msg)
			print "panic: " msg " at " frame
		}
	' "$1"
}

signature() {
	local sig
	sig="$(grep -m1 -o 'fuzz signature: .*' "$1" | sed 's/^fuzz signature: //' || true)"
	if [[ -z "$sig" ]]; then
		sig="$(crash_signature "$1")"
	fi
	if [[ -z "$sig" ]]; then
		sig="no signature: $(grep -m1 -E 'fuzzing process|deadline exceeded|timed out' "$1" | sed -E 's/[0-9]+/N/g; s/^[[:space:]]+//' || true)"
	fi
	printf '%s' "$sig"
}

hash_id() {
	if command -v sha256sum >/dev/null; then
		printf '%s' "$1" | sha256sum | cut -c1-12
	else
		printf '%s' "$1" | shasum -a 256 | cut -c1-12
	fi
}

file_finding() {
	local run_log="$1" input="$2" sig id dir count stamp
	sig="$(signature "$run_log")"
	id="$(hash_id "$sig")"
	dir="$FINDINGS/$id"
	stamp="$(date -u +%Y%m%dT%H%M%SZ)"

	mkdir -p "$dir/inputs"
	printf '%s\n' "$sig" >"$dir/signature"
	count="$(($(cat "$dir/count" 2>/dev/null || echo 0) + 1))"
	printf '%s\n' "$count" >"$dir/count"

	if (($(find "$dir/inputs" -type f | wc -l) < KEEP_INPUTS)); then
		mv "$CORPUS/$input" "$dir/inputs/$input"
		cp "$run_log" "$dir/$stamp.log"
	else
		rm "$CORPUS/$input"
	fi

	if ! grep -Fxq -- "$sig" "$SKIP_LIST" 2>/dev/null; then
		printf '%s\n' "$sig" >>"$SKIP_LIST"
		log "new failure $id: $sig"
	else
		log "known failure $id (hit $count times): $sig"
	fi
}

main() {
	local run_log="$FINDINGS/current.log" before after new errors=0 status

	touch "$SKIP_LIST"
	trap 'log "stopping"; exit 0' INT TERM

	log "fuzzing FuzzFullCLI in $CHUNK runs with $PARALLEL workers; findings in $FINDINGS"

	while true; do
		before="$(list_corpus)"

		status=0
		(cd "$REPO" && go_cmd test -run '^$' -fuzz '^FuzzFullCLI$' -fuzztime "$CHUNK" \
			-parallel "$PARALLEL" -timeout 0 ./test/) >"$run_log" 2>&1 || status=$?

		if ((status == 0)); then
			errors=0
			log "run finished clean: $(grep '^fuzz: elapsed' "$run_log" | tail -1)"
			continue
		fi

		after="$(list_corpus)"
		new="$(comm -13 <(printf '%s\n' "$before") <(printf '%s\n' "$after") | sed '/^$/d')"

		if [[ -z "$new" ]]; then
			errors=$((errors + 1))
			mkdir -p "$FINDINGS/errors"
			cp "$run_log" "$FINDINGS/errors/$(date -u +%Y%m%dT%H%M%SZ).log"
			log "run failed without a failing input ($errors/$MAX_ERRORS); see $FINDINGS/errors"
			if ((errors >= MAX_ERRORS)); then
				log "giving up after $MAX_ERRORS such failures in a row"
				exit 1
			fi
			sleep 60
			continue
		fi

		errors=0

		while IFS= read -r input; do
			file_finding "$run_log" "$input"
		done <<<"$new"
	done
}

main
