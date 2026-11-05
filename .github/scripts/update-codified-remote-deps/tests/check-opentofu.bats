#!/usr/bin/env bats

load helpers/common

setup() {
  common_setup
  install_stub mise

  SCRIPT="$BATS_TEST_DIRNAME/../check-opentofu.sh"

  cat >mise.toml <<'EOF'
[tools]
go = "1.27.3"
opentofu = "1.13.0"
golangci-lint = "2.14.0"
EOF
}

@test "reports an update when a newer release is out" {
  export STUB_RESPONSE="1.13.2"

  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q "^current=1.13.0$" "$GITHUB_OUTPUT"
  grep -q "^latest=1.13.2$" "$GITHUB_OUTPUT"
  grep -q "^needs_update=true$" "$GITHUB_OUTPUT"
}

@test "reports no update when the pinned release is the latest" {
  export STUB_RESPONSE="1.13.0"

  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q "^needs_update=false$" "$GITHUB_OUTPUT"
}

@test "asks mise for releases at least 7 days old" {
  export STUB_RESPONSE="1.13.2"

  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q "^latest --minimum-release-age 7d opentofu$" "$STUB_LOG"
}

@test "fails on a prerelease" {
  export STUB_RESPONSE="1.14.0-beta1"

  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"unexpected latest version: 1.14.0-beta1"* ]]
  [ ! -s "$GITHUB_OUTPUT" ]
}

@test "fails when mise fails" {
  export STUB_EXIT=1

  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [ ! -s "$GITHUB_OUTPUT" ]
}

@test "fails when mise.toml does not pin opentofu" {
  printf '[tools]\ngo = "1.27.3"\n' >mise.toml

  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"no pin matching"*"mise.toml"* ]]
}

@test "fails when GITHUB_OUTPUT is missing" {
  unset GITHUB_OUTPUT

  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"GITHUB_OUTPUT is not set"* ]]
}

@test "fails before any lookup when mise is missing" {
  run env PATH="$(only_tools_path grep tr)" "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"mise is required but not installed"* ]]
  [ ! -e "$STUB_LOG" ]
}
