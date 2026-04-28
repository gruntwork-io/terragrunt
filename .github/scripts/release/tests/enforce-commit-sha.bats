#!/usr/bin/env bats

setup() {
  SCRIPT="${BATS_TEST_DIRNAME}/../enforce-commit-sha.sh"
  VALID_SHA="a1b2c3d4e5f6789012345678901234567890abcd"
}

@test "accepts a valid 40-char lowercase hex SHA (arg)" {
  run "$SCRIPT" "$VALID_SHA"
  [ "$status" -eq 0 ]
}

@test "accepts a valid SHA via REF env var" {
  REF="$VALID_SHA" run "$SCRIPT"
  [ "$status" -eq 0 ]
}

@test "rejects empty ref" {
  run "$SCRIPT" ""
  [ "$status" -ne 0 ]
}

@test "rejects when neither arg nor REF is set" {
  run "$SCRIPT"
  [ "$status" -ne 0 ]
}

@test "rejects branch name" {
  run "$SCRIPT" "main"
  [ "$status" -ne 0 ]
}

@test "rejects tag-style ref" {
  run "$SCRIPT" "v1.2.3"
  [ "$status" -ne 0 ]
}

@test "rejects short SHA (7 chars)" {
  run "$SCRIPT" "a1b2c3d"
  [ "$status" -ne 0 ]
}

@test "rejects 39-char SHA" {
  run "$SCRIPT" "${VALID_SHA:0:39}"
  [ "$status" -ne 0 ]
}

@test "rejects 41-char SHA" {
  run "$SCRIPT" "${VALID_SHA}e"
  [ "$status" -ne 0 ]
}

@test "rejects uppercase hex" {
  run "$SCRIPT" "A1B2C3D4E5F6789012345678901234567890ABCD"
  [ "$status" -ne 0 ]
}

@test "rejects mixed case" {
  run "$SCRIPT" "A1b2c3d4e5f6789012345678901234567890abcd"
  [ "$status" -ne 0 ]
}

@test "rejects non-hex chars" {
  run "$SCRIPT" "g1b2c3d4e5f6789012345678901234567890abcd"
  [ "$status" -ne 0 ]
}

@test "rejects refs/heads/main" {
  run "$SCRIPT" "refs/heads/main"
  [ "$status" -ne 0 ]
}
