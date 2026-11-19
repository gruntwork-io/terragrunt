#!/usr/bin/env bash

set -euo pipefail

# Capture CPU and heap profiles of scenarios for the base and the head build,
# and a pprof diff between them. With [only-file], only the scenario ids that
# file lists are profiled.
#
# Usage: capture-profiles.sh <work-dir> <profiles-dir> [only-file]
#
# Run this after timing, never during it: profiling slows the process it
# measures. Written per scenario under <profiles-dir>/<scenario>/:
#   base/cpu.prof, base/mem.prof, head/cpu.prof, head/mem.prof
#   cpu-diff.txt   `pprof -top` of head against base, or a notice when a side
#                  has fewer than BENCH_PROFILE_MIN_SAMPLES CPU samples.
#   mem-diff.txt   `pprof -top` of head against base for allocated bytes.
#
# The CPU profiler samples 100 times a second and Terragrunt profiles the
# command it runs without its startup, so ten runs of `find` over 100 units
# record about one sample. Each profile is therefore merged from several runs:
# BENCH_PROFILE_RUNS to start with, then as many more as the samples so far
# predict it takes to reach BENCH_PROFILE_MIN_SAMPLES. A profile that would need
# more than BENCH_PROFILE_MAX_RUNS gets no more runs.
#
# Env:
#   BENCH_PROFILE_RUNS          Runs every profile starts with (default: 5).
#   BENCH_PROFILE_MIN_SAMPLES   CPU samples a side needs for its profile to be
#                               compared (default: 100).
#   BENCH_PROFILE_MAX_RUNS      Most runs merged into one profile (default: 400).
#   BENCH_SIZES                 See scenarios.sh.

readonly BENCH_PROFILE_RUNS="${BENCH_PROFILE_RUNS:-5}"
readonly BENCH_PROFILE_MIN_SAMPLES="${BENCH_PROFILE_MIN_SAMPLES:-100}"
readonly BENCH_PROFILE_MAX_RUNS="${BENCH_PROFILE_MAX_RUNS:-400}"
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

		diff_cpu_profiles "${profiles_dir}/${id}"
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
	local -r terragrunt="${work_dir}/bin/${side}/terragrunt"
	local fixture samples target
	local runs="$BENCH_PROFILE_RUNS"
	local -a args

	fixture="$(ensure_fixture "$work_dir" "$side" "$shape" "$units")"
	read -ra args <<<"$(scenario_args "$command")"

	mkdir -p "$out_dir"

	run_profiled "$terragrunt" "$fixture" "$out_dir" 1 "$runs" "${args[@]}"

	# A build that predates --profile-dir ignores TG_PROFILE_DIR.
	if [[ ! -f "${out_dir}/run-1/terragrunt_cpu.prof" ]]; then
		echo "The ${side} build wrote no profiles; skipping it" >&2
		rm -rf "${out_dir}"/run-*
		return 0
	fi

	# Each pass adds at least one run, and runs never pass BENCH_PROFILE_MAX_RUNS.
	while true; do
		samples="$(cpu_samples "${out_dir}"/run-*/terragrunt_cpu.prof)"
		target="$(runs_to_reach_min_samples "$samples" "$runs")"

		if ((target <= runs)); then
			break
		fi

		run_profiled "$terragrunt" "$fixture" "$out_dir" "$((runs + 1))" "$target" "${args[@]}"

		runs="$target"
	done

	go tool pprof -proto "${out_dir}"/run-*/terragrunt_cpu.prof >"${out_dir}/cpu.prof" 2>/dev/null
	go tool pprof -proto "${out_dir}"/run-*/terragrunt_mem.prof >"${out_dir}/mem.prof" 2>/dev/null

	rm -rf "${out_dir}"/run-*

	return 0
}

# Run a Terragrunt binary once for each run number from <first> to <last>, each
# run writing its profiles to <out-dir>/run-<number>.
#
# Usage: run_profiled <terragrunt> <fixture> <out-dir> <first> <last> <arg>...
function run_profiled {
	local -r terragrunt="$1"
	local -r fixture="$2"
	local -r out_dir="$3"
	local -r first="$4"
	local -r last="$5"
	local run

	shift 5

	for ((run = first; run <= last; run++)); do
		TG_PROFILE_DIR="${out_dir}/run-${run}" \
			"$terragrunt" "$@" --working-dir "$fixture" >/dev/null 2>&1
	done

	return 0
}

# Print the number of CPU samples in the given profiles, or 0 when the first of
# them does not exist.
function cpu_samples {
	local count

	if [[ ! -f "$1" ]]; then
		echo 0
		return 0
	fi

	count="$(
		go tool pprof -sample_index=samples -top -nodecount=1 "$@" 2>/dev/null |
			sed -n 's/.*Total samples = \([0-9][0-9]*\).*/\1/p'
	)"

	echo "${count:-0}"

	return 0
}

# Print how many runs a profile needs in all, given the CPU samples that <runs>
# runs recorded. Prints <runs> when they recorded enough, and also when reaching
# BENCH_PROFILE_MIN_SAMPLES would take more than BENCH_PROFILE_MAX_RUNS.
#
# Usage: runs_to_reach_min_samples <samples> <runs>
function runs_to_reach_min_samples {
	local -r samples="$1"
	local -r runs="$2"
	local target

	if ((samples == 0 || samples >= BENCH_PROFILE_MIN_SAMPLES)); then
		echo "$runs"
		return 0
	fi

	# The runs so far only estimate the sample rate, so this aims half again
	# past the minimum.
	target=$(((runs * BENCH_PROFILE_MIN_SAMPLES * 3 + samples * 2 - 1) / (samples * 2)))

	if ((target > BENCH_PROFILE_MAX_RUNS)); then
		echo "$runs"
		return 0
	fi

	echo "$target"

	return 0
}

# Write <scenario-dir>/cpu-diff.txt.
function diff_cpu_profiles {
	local -r scenario_dir="$1"
	local base_samples head_samples

	base_samples="$(cpu_samples "${scenario_dir}/base/cpu.prof")"
	head_samples="$(cpu_samples "${scenario_dir}/head/cpu.prof")"

	if ((base_samples < BENCH_PROFILE_MIN_SAMPLES || head_samples < BENCH_PROFILE_MIN_SAMPLES)); then
		echo "Too few CPU samples to compare: base ${base_samples}, head ${head_samples}, and each side needs ${BENCH_PROFILE_MIN_SAMPLES}." \
			>"${scenario_dir}/cpu-diff.txt"

		return 0
	fi

	diff_profiles "$scenario_dir" cpu

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
