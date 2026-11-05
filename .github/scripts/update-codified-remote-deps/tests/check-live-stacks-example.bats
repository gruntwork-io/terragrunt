#!/usr/bin/env bats

load helpers/common

readonly PINNED_SHA="3649bd95c93074e6a3742bc5122e411505c24c3a"
readonly NEWER_SHA="753aa3b43fd36a874c33ea67cb042a9fe51ca2e5"

setup() {
  common_setup
  install_stub git

  SCRIPT="$BATS_TEST_DIRNAME/../check-live-stacks-example.sh"

  mkdir -p test
  cat >test/integration_example_live_stacks_test.go <<EOF
	helpers.ExecWithTestLogger(
		t,
		repoDir,
		"git",
		"checkout",
		"$PINNED_SHA",
	)
EOF
}

@test "reports an update when HEAD moved past the pinned commit" {
  export STUB_RESPONSE="$NEWER_SHA	HEAD"

  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q "^current=$PINNED_SHA$" "$GITHUB_OUTPUT"
  grep -q "^latest=$NEWER_SHA$" "$GITHUB_OUTPUT"
  grep -q "^needs_update=true$" "$GITHUB_OUTPUT"
  grep -q "^ls-remote https://github.com/gruntwork-io/terragrunt-infrastructure-live-stacks-example.git HEAD$" "$STUB_LOG"
}

@test "reports no update when HEAD is the pinned commit" {
  export STUB_RESPONSE="$PINNED_SHA	HEAD"

  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q "^needs_update=false$" "$GITHUB_OUTPUT"
}

@test "finds the pin when checkout and the commit share a line" {
  printf '"git", "checkout", "%s")\n' "$PINNED_SHA" >test/integration_example_live_stacks_test.go
  export STUB_RESPONSE="$PINNED_SHA	HEAD"

  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q "^current=$PINNED_SHA$" "$GITHUB_OUTPUT"
}

@test "fails when ls-remote returns something other than a commit" {
  export STUB_RESPONSE="not-a-sha	HEAD"

  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"unexpected latest version: not-a-sha"* ]]
  [ ! -s "$GITHUB_OUTPUT" ]
}

@test "fails when ls-remote fails" {
  export STUB_EXIT=128

  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [ ! -s "$GITHUB_OUTPUT" ]
}

@test "fails when the test file has no pinned commit" {
  printf 'package test\n' >test/integration_example_live_stacks_test.go

  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"no pin matching"*"test/integration_example_live_stacks_test.go"* ]]
}

@test "fails when GITHUB_OUTPUT is missing" {
  unset GITHUB_OUTPUT

  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"GITHUB_OUTPUT is not set"* ]]
}

@test "fails before any lookup when git is missing" {
  run env PATH="$(only_tools_path awk grep tr)" "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"git is required but not installed"* ]]
  [ ! -e "$STUB_LOG" ]
}
