#!/usr/bin/env bash

set -euo pipefail

# Run a suite of the Go benchmarks in test/benchmarks from the base and the head
# checkout. Writes the raw output to <out-dir>/base.txt and <out-dir>/head.txt,
# and their benchstat comparison to <out-dir>/benchstat.txt.
#
# Usage: run-go-bench.sh <suite> <work-dir> <out-dir>
#
# <suite> is one of:
#   fast          BenchmarkScenario, which runs against faketofu.
#   integration   Every other benchmark. These run a real tofu, download
#                 providers, and clone remote sources, so they need the network.
#                 The RandomWait benchmarks are left out: each run draws its own
#                 delays, so two runs of one build already differ. The CAS
#                 benchmarks are left out because they fail: their fixture's
#                 __MIRROR_URL__ source is never pointed at a Git server.
#
# <work-dir> is the directory build.sh filled. Run from the head checkout.
#
# Each side runs its own copy of the benchmarks, so a base that predates a
# suite has nothing to run. In that case only head.txt is written.
#
# Env:
#   BENCH_GO_COUNT         Passes per side, alternating sides (default: 6, at least 4).
#   BENCH_GO_PROFILE_DIR   When set, one more pass per side writes CPU and heap
#                          profiles of the whole suite there, with pprof diffs.

readonly BENCH_GO_COUNT="${BENCH_GO_COUNT:-6}"
readonly BENCH_GO_PROFILE_DIR="${BENCH_GO_PROFILE_DIR:-}"
readonly BENCH_PACKAGE=test/benchmarks
readonly DIFF_NODE_COUNT=25
readonly MIN_PASSES=4

function main {
	local -r suite="${1:?Usage: run-go-bench.sh <fast|integration> <work-dir> <out-dir>}"
	local -r work_dir="${2:?Usage: run-go-bench.sh <fast|integration> <work-dir> <out-dir>}"
	local -r out_dir="${3:?Usage: run-go-bench.sh <fast|integration> <work-dir> <out-dir>}"
	local -r head_src="$PWD"
	local -r base_src="${work_dir}/src/base"
	local -a sides=(head)
	local -a filter
	local in_suite pass side

	case "$suite" in
	fast)
		filter=(-test.bench '^BenchmarkScenario$')
		in_suite='^BenchmarkScenario$'
		;;
	integration)
		filter=(-test.bench . -test.skip '^BenchmarkScenario$|RandomWait|^BenchmarkCAS')
		in_suite='^Benchmark'
		;;
	*)
		echo "Unknown suite: ${suite}" >&2
		return 1
		;;
	esac

	# compare.ts flags a metric at p < 0.05, which a rank test cannot reach with
	# fewer than four samples a side.
	if ((BENCH_GO_COUNT < MIN_PASSES)); then
		echo "BENCH_GO_COUNT must be at least ${MIN_PASSES}, got ${BENCH_GO_COUNT}" >&2
		return 1
	fi

	mkdir -p "$out_dir"

	go test -c -o "${work_dir}/bin/head/benchmarks.test" "./${BENCH_PACKAGE}"
	go -C "$base_src" test -c -o "${work_dir}/bin/base/benchmarks.test" "./${BENCH_PACKAGE}"

	# -test.list ignores -test.bench, so it takes the suite's own pattern.
	if [[ -n "$(run_pass "$base_src" "${work_dir}/bin/base/benchmarks.test" -test.list "$in_suite")" ]]; then
		sides=(base head)
	else
		echo "Base has no benchmarks in the ${suite} suite; running head only" >&2
	fi

	for ((pass = 1; pass <= BENCH_GO_COUNT; pass++)); do
		for side in "${sides[@]}"; do
			echo "Go benchmark pass ${pass} (${side})" >&2

			run_pass "$(side_source "$side" "$base_src" "$head_src")" \
				"${work_dir}/bin/${side}/benchmarks.test" "${filter[@]}" -test.benchmem \
				>>"${out_dir}/${side}.txt"
		done
	done

	if [[ -f "${out_dir}/base.txt" ]]; then
		benchstat "${out_dir}/base.txt" "${out_dir}/head.txt" >"${out_dir}/benchstat.txt"
	fi

	if [[ -n "$BENCH_GO_PROFILE_DIR" ]]; then
		profile_suite "$work_dir" "$base_src" "$head_src" "${sides[@]}" -- "${filter[@]}"
	fi

	return 0
}

function side_source {
	local -r side="$1"
	local -r base_src="$2"
	local -r head_src="$3"

	if [[ "$side" == "base" ]]; then
		echo "$base_src"
		return 0
	fi

	echo "$head_src"

	return 0
}

# Run a test binary from the benchmark package of a checkout. The benchmarks
# build faketofu with `go build`, which has to run inside the module the test
# binary came from.
function run_pass {
	local -r source_dir="$1"
	local -r test_binary="$2"

	shift 2

	(
		cd "${source_dir}/${BENCH_PACKAGE}"
		"$test_binary" -test.run '^$' -test.count 1 "$@"
	)

	return 0
}

# Usage: profile_suite <work-dir> <base-src> <head-src> <side>... -- <filter>...
function profile_suite {
	local -r work_dir="$1"
	local -r base_src="$2"
	local -r head_src="$3"

	shift 3

	local -a sides=()
	local side

	while [[ "$1" != "--" ]]; do
		sides+=("$1")
		shift
	done

	shift

	for side in "${sides[@]}"; do
		echo "Go benchmark profile pass (${side})" >&2
		mkdir -p "${BENCH_GO_PROFILE_DIR}/${side}"

		run_pass "$(side_source "$side" "$base_src" "$head_src")" \
			"${work_dir}/bin/${side}/benchmarks.test" "$@" \
			-test.benchtime 1x \
			-test.cpuprofile "${BENCH_GO_PROFILE_DIR}/${side}/cpu.prof" \
			-test.memprofile "${BENCH_GO_PROFILE_DIR}/${side}/mem.prof" \
			>/dev/null
	done

	if [[ "${#sides[@]}" -lt 2 ]]; then
		return 0
	fi

	go tool pprof -top -nodecount="$DIFF_NODE_COUNT" \
		-diff_base "${BENCH_GO_PROFILE_DIR}/base/cpu.prof" "${BENCH_GO_PROFILE_DIR}/head/cpu.prof" \
		>"${BENCH_GO_PROFILE_DIR}/cpu-diff.txt" 2>/dev/null

	go tool pprof -top -nodecount="$DIFF_NODE_COUNT" -sample_index=alloc_space \
		-diff_base "${BENCH_GO_PROFILE_DIR}/base/mem.prof" "${BENCH_GO_PROFILE_DIR}/head/mem.prof" \
		>"${BENCH_GO_PROFILE_DIR}/mem-diff.txt" 2>/dev/null

	return 0
}

main "$@"
