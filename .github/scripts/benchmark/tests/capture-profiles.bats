#!/usr/bin/env bats

setup() {
	SCRIPT="${BATS_TEST_DIRNAME}/../capture-profiles.sh"
	WORK="${BATS_TEST_TMPDIR}/work"
	PROFILES="${BATS_TEST_TMPDIR}/profiles"
	ONLY="${BATS_TEST_TMPDIR}/only"

	export RUN_LOG="${BATS_TEST_TMPDIR}/runs"
	export BENCH_PROFILE_RUNS=2
	export BENCH_PROFILE_MIN_SAMPLES=100
	export BENCH_PROFILE_MAX_RUNS=50

	echo "find.flat.1" >"$ONLY"

	mkdir -p "${WORK}/bin/base" "${WORK}/bin/head" "${WORK}/bin/tools" "${BATS_TEST_TMPDIR}/bin"

	# A terragrunt that logs its own path and writes empty profiles.
	cat >"${WORK}/bin/base/terragrunt" <<'STUB'
#!/usr/bin/env bash
echo "$0" >>"$RUN_LOG"
mkdir -p "$TG_PROFILE_DIR"
: >"${TG_PROFILE_DIR}/terragrunt_cpu.prof"
: >"${TG_PROFILE_DIR}/terragrunt_mem.prof"
STUB

	cp "${WORK}/bin/base/terragrunt" "${WORK}/bin/head/terragrunt"

	cat >"${WORK}/bin/tools/benchfixture" <<'STUB'
#!/usr/bin/env bash
mkdir -p "$2"
STUB

	# A go whose pprof merges profiles into the number of runs they hold, and
	# reports SAMPLES_PER_RUN samples for each of those runs.
	cat >"${BATS_TEST_TMPDIR}/bin/go" <<'STUB'
#!/usr/bin/env bash
runs=0

for arg in "$@"; do
	if [[ "$arg" != *.prof ]]; then
		continue
	fi

	held="$(cat "$arg")"
	runs=$((runs + ${held:-1}))
done

case "$*" in
*-sample_index=samples*)
	echo "Duration: 1s, Total samples = $((runs * SAMPLES_PER_RUN))"
	;;
*-proto*)
	echo "$runs"
	;;
*)
	echo "pprof diff"
	;;
esac
STUB

	chmod +x "${WORK}/bin/base/terragrunt" "${WORK}/bin/head/terragrunt" "${WORK}/bin/tools/benchfixture" "${BATS_TEST_TMPDIR}/bin/go"

	export PATH="${BATS_TEST_TMPDIR}/bin:${PATH}"
}

# Print how many times the terragrunt of a side ran.
runs_of() {
	grep -c "/bin/$1/terragrunt" "$RUN_LOG"
}

@test "keeps to the first runs when they record enough samples" {
	SAMPLES_PER_RUN=50 run "$SCRIPT" "$WORK" "$PROFILES" "$ONLY"
	[ "$status" -eq 0 ]

	[ "$(runs_of base)" -eq 2 ]
	[ "$(runs_of head)" -eq 2 ]
	[ "$(cat "${PROFILES}/find.flat.1/cpu-diff.txt")" = "pprof diff" ]
	[ "$(cat "${PROFILES}/find.flat.1/mem-diff.txt")" = "pprof diff" ]
}

@test "adds runs until a profile has enough samples" {
	SAMPLES_PER_RUN=10 run "$SCRIPT" "$WORK" "$PROFILES" "$ONLY"
	[ "$status" -eq 0 ]

	[ "$(runs_of base)" -eq 15 ]
	[ "$(runs_of head)" -eq 15 ]
	[ "$(cat "${PROFILES}/find.flat.1/cpu-diff.txt")" = "pprof diff" ]
}

@test "adds no runs when enough samples would take more than the most runs allowed" {
	SAMPLES_PER_RUN=1 run "$SCRIPT" "$WORK" "$PROFILES" "$ONLY"
	[ "$status" -eq 0 ]

	[ "$(runs_of base)" -eq 2 ]
	[ "$(cat "${PROFILES}/find.flat.1/cpu-diff.txt")" = "Too few CPU samples to compare: base 2, head 2, and each side needs 100." ]
	[ "$(cat "${PROFILES}/find.flat.1/mem-diff.txt")" = "pprof diff" ]
}

@test "profiles only the scenarios the only-file lists" {
	SAMPLES_PER_RUN=50 run "$SCRIPT" "$WORK" "$PROFILES" "$ONLY"
	[ "$status" -eq 0 ]

	[ "$(ls "$PROFILES")" = "find.flat.1" ]
}

@test "profiles nothing for an empty only-file" {
	: >"$ONLY"

	SAMPLES_PER_RUN=50 run "$SCRIPT" "$WORK" "$PROFILES" "$ONLY"
	[ "$status" -eq 0 ]

	[ ! -e "$PROFILES" ]
	[ ! -e "$RUN_LOG" ]
}
