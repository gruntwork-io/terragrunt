#!/usr/bin/env bats

load helpers/common

readonly DATA_FILE="docs/src/data/compatibility/compatibility.json"

setup() {
  common_setup
  install_stub git

  SCRIPT="$BATS_TEST_DIRNAME/../add-compatibility-entry.sh"

  export STUB_RESPONSE="aaaa	refs/tags/v1.3.0-rc1
bbbb	refs/tags/v1.2.9
cccc	refs/tags/v1.2.8"

  mkdir -p "$(dirname "$DATA_FILE")"
  cat >"$DATA_FILE" <<'EOF'
[
  {
    "id": "opentofu-1.13.x",
    "tool": "opentofu",
    "version": "1.13.x",
    "terragrunt_min": "1.2.0",
    "terragrunt_max": null,
    "order": 28
  },
  {
    "id": "terraform-1.17.x",
    "tool": "terraform",
    "version": "1.17.x",
    "terragrunt_min": "1.2.2",
    "terragrunt_max": null,
    "order": 21
  },
  {
    "id": "terraform-1.14.x",
    "tool": "terraform",
    "version": "1.14.x",
    "terragrunt_min": "0.94.0",
    "terragrunt_max": null,
    "order": 20
  }
]
EOF
  cp "$DATA_FILE" original.json
}

@test "adds a new Terraform minor above the other Terraform entries" {
  run "$SCRIPT" terraform 1.18.0
  [ "$status" -eq 0 ]

  cat >expected.json <<'EOF'
[
  {
    "id": "opentofu-1.13.x",
    "tool": "opentofu",
    "version": "1.13.x",
    "terragrunt_min": "1.2.0",
    "terragrunt_max": null,
    "order": 28
  },
  {
    "id": "terraform-1.18.x",
    "tool": "terraform",
    "version": "1.18.x",
    "terragrunt_min": "1.2.10",
    "terragrunt_max": null,
    "order": 22
  },
  {
    "id": "terraform-1.17.x",
    "tool": "terraform",
    "version": "1.17.x",
    "terragrunt_min": "1.2.2",
    "terragrunt_max": null,
    "order": 21
  },
  {
    "id": "terraform-1.14.x",
    "tool": "terraform",
    "version": "1.14.x",
    "terragrunt_min": "0.94.0",
    "terragrunt_max": null,
    "order": 20
  }
]
EOF
  diff expected.json "$DATA_FILE"
}

@test "adds a new OpenTofu minor at the top of the file" {
  run "$SCRIPT" opentofu 1.14.1
  [ "$status" -eq 0 ]
  [ "$(jq -c '.[0]' "$DATA_FILE")" = '{"id":"opentofu-1.14.x","tool":"opentofu","version":"1.14.x","terragrunt_min":"1.2.10","terragrunt_max":null,"order":29}' ]
  [ "$(jq length "$DATA_FILE")" -eq 4 ]
}

@test "reports the entry and its terragrunt_min in the note" {
  run "$SCRIPT" terraform 1.18.0
  [ "$status" -eq 0 ]
  grep -q '^note=Adds `terraform-1.18.x` to .* with `terragrunt_min` `1.2.10`, the patch release after `v1.2.9`\.' "$GITHUB_OUTPUT"
}

@test "skips prerelease tags when finding the latest Terragrunt release" {
  run "$SCRIPT" terraform 1.18.0
  [ "$status" -eq 0 ]
  grep -q "^ls-remote --tags --refs --sort=-v:refname https://github.com/gruntwork-io/terragrunt.git v\*$" "$STUB_LOG"
  [ "$(jq -r '.[1].terragrunt_min' "$DATA_FILE")" = "1.2.10" ]
}

@test "leaves the file alone when the minor is already listed" {
  run "$SCRIPT" terraform 1.17.4
  [ "$status" -eq 0 ]
  diff original.json "$DATA_FILE"
  grep -q "^note=$" "$GITHUB_OUTPUT"
  [ ! -e "$STUB_LOG" ]
}

@test "fails on an unknown tool" {
  run "$SCRIPT" pulumi 1.18.0
  [ "$status" -ne 0 ]
  [[ "$output" == *"unexpected tool: pulumi"* ]]
  diff original.json "$DATA_FILE"
}

@test "fails on a prerelease" {
  run "$SCRIPT" terraform 1.18.0-alpha20261101
  [ "$status" -ne 0 ]
  [[ "$output" == *"unexpected version: 1.18.0-alpha20261101"* ]]
  diff original.json "$DATA_FILE"
}

@test "fails without both arguments" {
  run "$SCRIPT" terraform
  [ "$status" -ne 0 ]
  [[ "$output" == *"usage: add-compatibility-entry.sh"* ]]
}

@test "fails when ls-remote fails" {
  export STUB_EXIT=128

  run "$SCRIPT" terraform 1.18.0
  [ "$status" -ne 0 ]
  diff original.json "$DATA_FILE"
  [ ! -s "$GITHUB_OUTPUT" ]
}

@test "fails when no release tag is found" {
  export STUB_RESPONSE="aaaa	refs/tags/v1.3.0-rc1"

  run "$SCRIPT" terraform 1.18.0
  [ "$status" -ne 0 ]
  [[ "$output" == *"no release tag found"* ]]
  diff original.json "$DATA_FILE"
}

@test "fails when GITHUB_OUTPUT is missing" {
  unset GITHUB_OUTPUT

  run "$SCRIPT" terraform 1.18.0
  [ "$status" -ne 0 ]
  [[ "$output" == *"GITHUB_OUTPUT is not set"* ]]
}

@test "fails before any lookup when jq is missing" {
  run env PATH="$(only_tools_path git grep mktemp)" "$SCRIPT" terraform 1.18.0
  [ "$status" -ne 0 ]
  [[ "$output" == *"jq is required but not installed"* ]]
  [ ! -e "$STUB_LOG" ]
}
