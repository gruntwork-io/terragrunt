#!/usr/bin/env bats

setup() {
  SCRIPT="${BATS_TEST_DIRNAME}/../check-release-exists.sh"
  STUB_DIR="$(mktemp -d)"
  cp "${BATS_TEST_DIRNAME}/helpers/gh-stub.sh" "${STUB_DIR}/gh"
  chmod +x "${STUB_DIR}/gh"
  export PATH="${STUB_DIR}:${PATH}"

  GITHUB_OUTPUT="$(mktemp)"
  export GITHUB_OUTPUT
  export GH_TOKEN=fake
  export VERSION=v1.0.0
}

teardown() {
  rm -rf "$STUB_DIR"
  rm -f "$GITHUB_OUTPUT"
}

@test "writes exists=false and exits 0 when no release found" {
  export GH_STUB_RESPONSE=""
  export GH_STUB_EXIT=1

  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q "^exists=false$" "$GITHUB_OUTPUT"
  grep -q "^is_draft=false$" "$GITHUB_OUTPUT"
  grep -q "^release_id=$" "$GITHUB_OUTPUT"
}

@test "reports draft release" {
  export GH_STUB_RESPONSE='{"id":123,"uploadUrl":"https://uploads/repos/o/r/releases/123/assets{?name,label}","isDraft":true}'

  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q "^exists=true$" "$GITHUB_OUTPUT"
  grep -q "^release_id=123$" "$GITHUB_OUTPUT"
  grep -q "^is_draft=true$" "$GITHUB_OUTPUT"
}

@test "reports non-draft (published) release" {
  export GH_STUB_RESPONSE='{"id":456,"uploadUrl":"https://uploads/repos/o/r/releases/456/assets{?name,label}","isDraft":false}'

  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q "^exists=true$" "$GITHUB_OUTPUT"
  grep -q "^release_id=456$" "$GITHUB_OUTPUT"
  grep -q "^is_draft=false$" "$GITHUB_OUTPUT"
}

@test "fails when VERSION missing" {
  unset VERSION
  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"VERSION"* ]]
}

@test "fails when GH_TOKEN missing" {
  unset GH_TOKEN
  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"GH_TOKEN"* ]]
}

@test "fails when GITHUB_OUTPUT missing" {
  unset GITHUB_OUTPUT
  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"GITHUB_OUTPUT"* ]]
}
