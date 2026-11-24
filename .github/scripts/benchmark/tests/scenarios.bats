#!/usr/bin/env bats

setup() {
	LIB="${BATS_TEST_DIRNAME}/../scenarios.sh"
}

@test "scenario_ids lists every command, shape, and size" {
	run bash -c "source '$LIB' && scenario_ids"
	[ "$status" -eq 0 ]
	[ "${#lines[@]}" -eq 24 ]
	[ "${lines[0]}" = "find.flat.1" ]
	[ "${lines[23]}" = "run-all-plan.deps.1000" ]
}

@test "scenario_ids honors BENCH_SIZES" {
	BENCH_SIZES="1 10" run bash -c "source '$LIB' && scenario_ids"
	[ "$status" -eq 0 ]
	[ "${#lines[@]}" -eq 12 ]
}

@test "scenario_ids prints only the ids an only-file lists" {
	printf 'find.deps.100\nrun-all-plan.flat.1\n' >"${BATS_TEST_TMPDIR}/only"

	run bash -c "source '$LIB' && scenario_ids '${BATS_TEST_TMPDIR}/only'"
	[ "$status" -eq 0 ]
	[ "${#lines[@]}" -eq 2 ]
	[ "${lines[0]}" = "find.deps.100" ]
	[ "${lines[1]}" = "run-all-plan.flat.1" ]
}

@test "scenario_ids prints nothing for an empty only-file" {
	: >"${BATS_TEST_TMPDIR}/only"

	run bash -c "source '$LIB' && scenario_ids '${BATS_TEST_TMPDIR}/only'"
	[ "$status" -eq 0 ]
	[ -z "$output" ]
}

@test "scenario_ids does not match an id that is a prefix of a listed one" {
	echo "find.flat.1000" >"${BATS_TEST_TMPDIR}/only"

	run bash -c "source '$LIB' && scenario_ids '${BATS_TEST_TMPDIR}/only'"
	[ "$status" -eq 0 ]
	[ "$output" = "find.flat.1000" ]
}

@test "scenario_args rejects an unknown command" {
	run bash -c "source '$LIB' && scenario_args destroy"
	[ "$status" -eq 1 ]
	[[ "$output" == *"Unknown benchmark command: destroy"* ]]
}
