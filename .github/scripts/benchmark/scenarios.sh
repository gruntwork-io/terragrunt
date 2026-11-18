#!/usr/bin/env bash

# Scenario definitions shared by run-hyperfine.sh and capture-profiles.sh, which
# source this file. A scenario id is <command>.<shape>.<units>, and
# BenchmarkScenario in test/benchmarks runs the same set in process.
#
# Env:
#   BENCH_SIZES   Unit counts to run, space-separated (default: "1 10 100 1000").

readonly BENCH_COMMANDS=(find find-dependencies run-all-plan)
readonly BENCH_SHAPES=(flat deps)
readonly BENCH_SIZES="${BENCH_SIZES:-1 10 100 1000}"

# Print the scenario ids to run, one per line. With a file argument, print only
# the ids that file lists.
function scenario_ids {
	local -r only_file="${1:-}"
	local -a sizes

	read -ra sizes <<<"$BENCH_SIZES"

	printf '%s\n' "${BENCH_COMMANDS[@]}" |
		append_each "${BENCH_SHAPES[@]}" |
		append_each "${sizes[@]}" |
		keep_listed "$only_file"
}

# Print the Terragrunt arguments of a command, without --working-dir.
function scenario_args {
	local -r command="$1"

	case "$command" in
	find)
		echo "find"
		;;
	find-dependencies)
		echo "find --json --dependencies"
		;;
	run-all-plan)
		echo "run --all plan --non-interactive"
		;;
	*)
		echo "Unknown benchmark command: ${command}" >&2
		return 1
		;;
	esac

	return 0
}

# Write the fixture for one side of a scenario unless it exists, and print its
# path. Each side gets its own copy so the two builds never share a
# .terragrunt-cache.
function ensure_fixture {
	local -r work_dir="$1"
	local -r side="$2"
	local -r shape="$3"
	local -r units="$4"
	local -r dir="${work_dir}/fixtures/${side}/${shape}-${units}"

	if [[ ! -d "$dir" ]]; then
		"${work_dir}/bin/tools/benchfixture" -dir "$dir" -shape "$shape" -units "$units"
	fi

	echo "$dir"

	return 0
}

# Print each stdin line once per argument, with "." and the argument appended.
function append_each {
	local prefix suffix

	while IFS= read -r prefix; do
		for suffix in "$@"; do
			echo "${prefix}.${suffix}"
		done
	done

	return 0
}

# Copy the stdin lines that the file lists to stdout. With no file, copy them
# all.
function keep_listed {
	local -r file="$1"

	if [[ -z "$file" ]]; then
		cat
		return 0
	fi

	# grep exits 1 when no line matches.
	grep -xFf "$file" || (($? == 1))
}
