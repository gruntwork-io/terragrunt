#!/usr/bin/env bats

# Tests for resolve-version-ref.sh.
# Stubs the `gh` CLI via PATH so tests run hermetically.

setup() {
  SCRIPT="${BATS_TEST_DIRNAME}/../resolve-version-ref.sh"
  STUB_DIR="$(mktemp -d)"
  cp "${BATS_TEST_DIRNAME}/helpers/gh-stub.sh" "${STUB_DIR}/gh"
  chmod +x "${STUB_DIR}/gh"
  export PATH="${STUB_DIR}:${PATH}"

  GITHUB_OUTPUT="$(mktemp)"
  export GITHUB_OUTPUT
  export INPUT_VERSION=v1.2.3
  export GH_TOKEN=fake
}

teardown() {
  rm -rf "$STUB_DIR"
  rm -f "$GITHUB_OUTPUT"
}

@test "resolves draft release to version + ref" {
  local sha="a1b2c3d4e5f6789012345678901234567890abcd"
  export GH_STUB_RESPONSE="{\"isDraft\":true,\"targetCommitish\":\"${sha}\"}"

  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q "^version=${INPUT_VERSION}$" "$GITHUB_OUTPUT"
  grep -q "^ref=${sha}$" "$GITHUB_OUTPUT"
}

@test "errors if no release found" {
  export GH_STUB_RESPONSE=""
  export GH_STUB_EXIT=1

  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"No release found"* ]]
}

@test "resolves published release to version + ref" {
  local sha="a1b2c3d4e5f6789012345678901234567890abcd"
  export GH_STUB_RESPONSE="{\"targetCommitish\":\"${sha}\"}"

  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q "^version=${INPUT_VERSION}$" "$GITHUB_OUTPUT"
  grep -q "^ref=${sha}$" "$GITHUB_OUTPUT"
}

@test "fails when INPUT_VERSION missing" {
  unset INPUT_VERSION
  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"INPUT_VERSION"* ]]
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
