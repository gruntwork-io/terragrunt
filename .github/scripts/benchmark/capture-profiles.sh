#!/usr/bin/env bash

set -euo pipefail

# Capture CPU and heap profiles of every scenario for the base and the head
# build, and a pprof diff between them.
#
# Usage: capture-profiles.sh <work-dir> <profiles-dir> [only-file]
#
# Run this after timing, never during it: profiling slows the process it
# measures. Written per scenario under <profiles-dir>/<scenario>/:
#   base/cpu.prof, base/mem.prof, head/cpu.prof, head/mem.prof
#   cpu-diff.txt   `pprof -top` of head against base.
#   mem-diff.txt   The same for allocated bytes.
#
# The CPU profiler samples 100 times a second, so one run of a 30 ms scenario
# records about three samples. Each profile is therefore merged from several
# runs.
#
# Env:
#   BENCH_PROFILE_RUNS   Runs merged into each profile (default: 5).
#   BENCH_SIZES          See scenarios.sh.

readonly BENCH_PROFILE_RUNS="${BENCH_PROFILE_RUNS:-5}"
readonly DIFF_NODE_COUNT=25

# shellcheck source=scenarios.sh
source "$(dirname "${BASH_SOURCE[0]}")/scenarios.sh"

function main {
	local -r work_dir="${1:?Usage: capture-profiles.sh <work-dir> <profiles-dir> [only-file]}"
	local -r profiles_dir="${2:?Usage: capture-profiles.sh <work-dir> <profiles-dir> [only-file]}"
	local -r only_file="${3:-}"
	local id command shape units side

	export PATH="${work_dir}/bin/fake:${PATH}"

	while IFS= read -r id; do
		IFS=. read -r command shape units <<<"$id"

		echo "Profiling ${id}" >&2

		for side in base head; do
			profile_side "$work_dir" "$side" "$command" "$shape" "$units" "${profiles_dir}/${id}/${side}"
		done

		diff_profiles "${profiles_dir}/${id}" cpu
		diff_profiles "${profiles_dir}/${id}" mem -sample_index=alloc_space
	done < <(scenario_ids "$only_file")

	return 0
}

function profile_side {
	local -r work_dir="$1"
	local -r side="$2"
	local -r command="$3"
	local -r shape="$4"
	local -r units="$5"
	local -r out_dir="$6"
	local fixture run
	local -a args

	fixture="$(ensure_fixture "$work_dir" "$side" "$shape" "$units")"
	read -ra args <<<"$(scenario_args "$command")"

	mkdir -p "$out_dir"

	for ((run = 1; run <= BENCH_PROFILE_RUNS; run++)); do
		TG_PROFILE_DIR="${out_dir}/run-${run}" \
			"${work_dir}/bin/${side}/terragrunt" "${args[@]}" --working-dir "$fixture" >/dev/null 2>&1
	done

	# A build that predates --profile-dir ignores TG_PROFILE_DIR.
	if [[ ! -f "${out_dir}/run-1/terragrunt_cpu.prof" ]]; then
		echo "The ${side} build wrote no profiles; skipping it" >&2
		rm -rf "${out_dir}"/run-*
		return 0
	fi

	go tool pprof -proto "${out_dir}"/run-*/terragrunt_cpu.prof >"${out_dir}/cpu.prof" 2>/dev/null
	go tool pprof -proto "${out_dir}"/run-*/terragrunt_mem.prof >"${out_dir}/mem.prof" 2>/dev/null

	rm -rf "${out_dir}"/run-*

	return 0
}

# Write <scenario-dir>/<kind>-diff.txt. Extra arguments go to pprof.
function diff_profiles {
	local -r scenario_dir="$1"
	local -r kind="$2"

	shift 2

	# pprof exits non-zero when a profile has no samples, which a short scenario
	# can still produce.
	if ! go tool pprof -top -nodecount="$DIFF_NODE_COUNT" "$@" \
		-diff_base "${scenario_dir}/base/${kind}.prof" "${scenario_dir}/head/${kind}.prof" \
		>"${scenario_dir}/${kind}-diff.txt" 2>/dev/null; then
		echo "No samples to compare." >"${scenario_dir}/${kind}-diff.txt"
	fi

	return 0
}

main "$@"
