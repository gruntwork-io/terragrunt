#!/usr/bin/env bats

setup() {
  SCRIPT="${BATS_TEST_DIRNAME}/../validate-semver.sh"
}

@test "accepts v1.0.0" {
  run "$SCRIPT" v1.0.0
  [ "$status" -eq 0 ]
}

@test "accepts v0.0.0" {
  run "$SCRIPT" v0.0.0
  [ "$status" -eq 0 ]
}

@test "accepts v10.20.30" {
  run "$SCRIPT" v10.20.30
  [ "$status" -eq 0 ]
}

@test "accepts v1.0.0-rc1" {
  run "$SCRIPT" v1.0.0-rc1
  [ "$status" -eq 0 ]
}

@test "accepts v1.0.0-alpha.1" {
  run "$SCRIPT" v1.0.0-alpha.1
  [ "$status" -eq 0 ]
}

@test "accepts v1.0.0-alpha.1.2" {
  run "$SCRIPT" v1.0.0-alpha.1.2
  [ "$status" -eq 0 ]
}

@test "accepts v1.2.3+build (build metadata)" {
  run "$SCRIPT" v1.2.3+build
  [ "$status" -eq 0 ]
}

@test "accepts v1.2.3+exp.sha.5114f85" {
  run "$SCRIPT" v1.2.3+exp.sha.5114f85
  [ "$status" -eq 0 ]
}

@test "accepts v1.2.3-rc.1+build.42" {
  run "$SCRIPT" v1.2.3-rc.1+build.42
  [ "$status" -eq 0 ]
}

@test "accepts hyphen in pre-release identifier" {
  run "$SCRIPT" v1.2.3-x-y-z
  [ "$status" -eq 0 ]
}

@test "reads VERSION env var when no arg" {
  VERSION=v1.2.3 run "$SCRIPT"
  [ "$status" -eq 0 ]
}

@test "rejects empty string" {
  run "$SCRIPT" ""
  [ "$status" -ne 0 ]
}

@test "rejects missing version" {
  run "$SCRIPT"
  [ "$status" -ne 0 ]
}

@test "rejects 1.0.0 (no v prefix)" {
  run "$SCRIPT" 1.0.0
  [ "$status" -ne 0 ]
}

@test "rejects V1.0.0 (uppercase v)" {
  run "$SCRIPT" V1.0.0
  [ "$status" -ne 0 ]
}

@test "rejects v1 (incomplete)" {
  run "$SCRIPT" v1
  [ "$status" -ne 0 ]
}

@test "rejects v1.2 (missing patch)" {
  run "$SCRIPT" v1.2
  [ "$status" -ne 0 ]
}

@test "rejects v01.2.3 (leading zero major)" {
  run "$SCRIPT" v01.2.3
  [ "$status" -ne 0 ]
}

@test "rejects v1.02.3 (leading zero minor)" {
  run "$SCRIPT" v1.02.3
  [ "$status" -ne 0 ]
}

@test "rejects v1.2.03 (leading zero patch)" {
  run "$SCRIPT" v1.2.03
  [ "$status" -ne 0 ]
}

@test "rejects v1.2.3- (empty prerelease)" {
  run "$SCRIPT" v1.2.3-
  [ "$status" -ne 0 ]
}

@test "rejects v1.2.3+ (empty build metadata)" {
  run "$SCRIPT" v1.2.3+
  [ "$status" -ne 0 ]
}

@test "rejects v1.2.3+build..1 (empty build identifier)" {
  run "$SCRIPT" v1.2.3+build..1
  [ "$status" -ne 0 ]
}

@test "rejects alpha2025022501 (date-style)" {
  run "$SCRIPT" alpha2025022501
  [ "$status" -ne 0 ]
}

@test "rejects v1.2.3-RC_1 (underscore)" {
  run "$SCRIPT" v1.2.3-RC_1
  [ "$status" -ne 0 ]
}

@test "rejects leading whitespace" {
  run "$SCRIPT" " v1.2.3"
  [ "$status" -ne 0 ]
}
