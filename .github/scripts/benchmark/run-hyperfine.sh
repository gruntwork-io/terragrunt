#!/usr/bin/env bash

set -euo pipefail

# Time every scenario with the base and the head build and write one hyperfine
# JSON export per side and round to <results-dir>/<scenario>/<side>-<round>.json.
#
# Usage: run-hyperfine.sh <work-dir> <results-dir> [only-file]
#
# <work-dir> is the directory build.sh filled. With [only-file], only the
# scenario ids that file lists run.
#
# hyperfine runs one command to completion before the next, so a slow minute on
# a shared runner would land on one side only. Timing in short rounds that
# alternate which side goes first spreads that noise over both.
#
# Env:
#   BENCH_ROUNDS           Rounds per scenario (default: 5).
#   BENCH_RUNS_PER_ROUND   Timed runs per side in each round (default: 3).
#   BENCH_SIZES            See scenarios.sh.

readonly BENCH_ROUNDS="${BENCH_ROUNDS:-5}"
readonly BENCH_RUNS_PER_ROUND="${BENCH_RUNS_PER_ROUND:-3}"

# shellcheck source=scenarios.sh
source "$(dirname "${BASH_SOURCE[0]}")/scenarios.sh"

function main {
	local -r work_dir="${1:?Usage: run-hyperfine.sh <work-dir> <results-dir> [only-file]}"
	local -r results_dir="${2:?Usage: run-hyperfine.sh <work-dir> <results-dir> [only-file]}"
	local -r only_file="${3:-}"
	local id command shape units round side
	local -a sides

	# Terragrunt asks tofu for its version even in commands that never run it.
	export PATH="${work_dir}/bin/fake:${PATH}"

	mkdir -p "$results_dir"

	while IFS= read -r id; do
		IFS=. read -r command shape units <<<"$id"

		echo "Timing ${id}" >&2
		mkdir -p "${results_dir}/${id}"

		for ((round = 1; round <= BENCH_ROUNDS; round++)); do
			sides=(base head)

			if ((round % 2 == 0)); then
				sides=(head base)
			fi

			for side in "${sides[@]}"; do
				time_side "$work_dir" "$side" "$command" "$shape" "$units" "${results_dir}/${id}/${side}-${round}.json"
			done
		done
	done < <(scenario_ids "$only_file")

	return 0
}

function time_side {
	local -r work_dir="$1"
	local -r side="$2"
	local -r command="$3"
	local -r shape="$4"
	local -r units="$5"
	local -r export_file="$6"
	local fixture args

	fixture="$(ensure_fixture "$work_dir" "$side" "$shape" "$units")"
	args="$(scenario_args "$command")"

	# hyperfine warns on stderr about every outlier it sees, which on a shared
	# runner is most rounds, so its stderr is shown only when it fails.
	if ! hyperfine \
		--style none \
		--warmup 1 \
		--runs "$BENCH_RUNS_PER_ROUND" \
		--export-json "$export_file" \
		"${work_dir}/bin/${side}/terragrunt ${args} --working-dir ${fixture}" \
		2>"${export_file}.stderr"; then
		cat "${export_file}.stderr" >&2
		return 1
	fi

	rm -f "${export_file}.stderr"

	return 0
}

main "$@"
