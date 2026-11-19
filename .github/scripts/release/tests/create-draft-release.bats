#!/usr/bin/env bats

# Tests for create-draft-release.sh.
# Stubs the `gh` CLI via PATH so tests run hermetically.

setup() {
  SCRIPT="${BATS_TEST_DIRNAME}/../create-draft-release.sh"
  STUB_DIR="$(mktemp -d)"
  cp "${BATS_TEST_DIRNAME}/helpers/gh-stub.sh" "${STUB_DIR}/gh"
  chmod +x "${STUB_DIR}/gh"
  export PATH="${STUB_DIR}:${PATH}"

  export GH_STUB_LOG="${BATS_TEST_TMPDIR}/gh.log"
  export NOTES_FILE="${BATS_TEST_TMPDIR}/notes.md"
  printf '## Bug Fixes\n' >"$NOTES_FILE"

  export VERSION=v1.2.3
  export TARGET=a1b2c3d4e5f6789012345678901234567890abcd
  export GH_TOKEN=fake
}

teardown() {
  rm -rf "$STUB_DIR"
}

@test "creates a draft release targeting the commit" {
  run "$SCRIPT"
  [ "$status" -eq 0 ]
  [ "$(cat "$GH_STUB_LOG")" = "release create v1.2.3 --draft --title v1.2.3 --target ${TARGET} --notes-file ${NOTES_FILE}" ]
  [[ "$output" == *"Created draft release v1.2.3"* ]]
}

@test "marks a version with a pre-release suffix as a pre-release" {
  export VERSION=v1.2.3-rc1
  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q -- "^release create v1.2.3-rc1 --draft .* --prerelease$" "$GH_STUB_LOG"
  [[ "$output" == *"Created draft pre-release v1.2.3-rc1"* ]]
}

@test "does not mark build metadata as a pre-release" {
  export VERSION=v1.2.3+build-5
  run "$SCRIPT"
  [ "$status" -eq 0 ]
  ! grep -q -- "--prerelease" "$GH_STUB_LOG"
}

@test "fails when gh cannot create the release" {
  export GH_STUB_EXIT=1
  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" != *"Created draft"* ]]
}

@test "fails when the notes file is empty" {
  : >"$NOTES_FILE"
  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"missing or empty"* ]]
  [ ! -e "$GH_STUB_LOG" ]
}

@test "fails when VERSION missing" {
  unset VERSION
  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"VERSION"* ]]
}

@test "fails when TARGET missing" {
  unset TARGET
  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"TARGET"* ]]
}

@test "fails when NOTES_FILE missing" {
  unset NOTES_FILE
  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"NOTES_FILE"* ]]
}

@test "fails when GH_TOKEN missing" {
  unset GH_TOKEN
  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"GH_TOKEN"* ]]
}
