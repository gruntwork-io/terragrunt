#!/usr/bin/env bats

load helpers/common

setup() {
  common_setup
  install_stub mise

  SCRIPT="$BATS_TEST_DIRNAME/../check-terraform.sh"

  mkdir -p .github/scripts/setup
  cat >.github/scripts/setup/terraform-switch-latest.sh <<'EOF'
#!/usr/bin/env bash

set -euo pipefail

# Install Terraform 1.14.4
.github/scripts/setup/terraform-switch.sh 1.14.4
EOF
}

@test "reports an update when a newer release is out" {
  export STUB_RESPONSE="1.17.0"

  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q "^current=1.14.4$" "$GITHUB_OUTPUT"
  grep -q "^latest=1.17.0$" "$GITHUB_OUTPUT"
  grep -q "^needs_update=true$" "$GITHUB_OUTPUT"
}

@test "reports no update when the pinned release is the latest" {
  export STUB_RESPONSE="1.14.4"

  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q "^needs_update=false$" "$GITHUB_OUTPUT"
}

@test "asks mise for releases at least 7 days old" {
  export STUB_RESPONSE="1.17.0"

  run "$SCRIPT"
  [ "$status" -eq 0 ]
  grep -q "^latest --minimum-release-age 7d terraform$" "$STUB_LOG"
}

@test "fails on a prerelease" {
  export STUB_RESPONSE="1.18.0-alpha20261101"

  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"unexpected latest version: 1.18.0-alpha20261101"* ]]
  [ ! -s "$GITHUB_OUTPUT" ]
}

@test "fails when mise fails" {
  export STUB_EXIT=1

  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [ ! -s "$GITHUB_OUTPUT" ]
}

@test "fails when the switch script has no pinned version" {
  printf '#!/usr/bin/env bash\n' >.github/scripts/setup/terraform-switch-latest.sh

  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"no pin matching"*"terraform-switch-latest.sh"* ]]
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
