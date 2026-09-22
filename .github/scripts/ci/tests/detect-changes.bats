#!/usr/bin/env bats

# Required for `run --separate-stderr`, which keeps the fallback notice out of $output.
bats_require_minimum_version 1.5.0

setup() {
  # Keep any summary a script writes inside this test's tmpdir, not in the job summary.
  export GITHUB_STEP_SUMMARY="${BATS_TEST_TMPDIR}/summary.md"

  SCRIPT="${BATS_TEST_DIRNAME}/../detect-changes.sh"

  cd "$BATS_TEST_TMPDIR"
  git init --quiet --initial-branch=main .
  git config user.email "ci@example.com"
  git config user.name "CI"
  git config commit.gpgsign false

  commit README.md
  git update-ref refs/remotes/origin/main HEAD
  BASE="$(git rev-parse HEAD)"

  export GITHUB_REF="refs/heads/feature"
}

# Commit an edit to each path given.
commit() {
  local path

  for path in "$@"; do
    mkdir -p "$(dirname "$path")"
    echo "$RANDOM" >>"$path"
    git add "$path"
  done

  git commit --quiet --message "edit $*"
}

@test "docs-only branch skips code and install jobs" {
  commit docs/src/content/docs/index.mdx CONTRIBUTING.md
  run --separate-stderr "$SCRIPT" ""
  [ "$status" -eq 0 ]
  [ "$output" = $'code=false\ninstall=false' ]
}

@test "Go change runs code jobs" {
  commit internal/cli/app.go
  run --separate-stderr "$SCRIPT" ""
  [ "$output" = $'code=true\ninstall=false' ]
}

@test "nested Markdown is test input, so it runs code jobs" {
  commit test/fixtures/catalog/README.md
  run --separate-stderr "$SCRIPT" ""
  [[ "$output" == *"code=true"* ]]
}

@test "docs fixtures run code jobs" {
  commit docs/src/fixtures/terralith-to-terragrunt/app/main.tf
  run --separate-stderr "$SCRIPT" ""
  [[ "$output" == *"code=true"* ]]
}

@test "install script change runs install and code jobs" {
  commit docs/public/install
  run --separate-stderr "$SCRIPT" ""
  [ "$output" = $'code=true\ninstall=true' ]
}

@test "docs follow-up on a branch with code changes still runs code jobs" {
  commit internal/cli/app.go
  commit docs/src/content/docs/index.mdx
  run --separate-stderr "$SCRIPT" "$(git rev-parse HEAD~1)"
  [[ "$output" == *"code=true"* ]]
}

@test "on main, only the pushed commits count" {
  commit internal/cli/app.go
  local before
  before="$(git rev-parse HEAD)"
  commit docs/src/content/docs/index.mdx

  GITHUB_REF="refs/heads/main" run --separate-stderr "$SCRIPT" "$before"
  [ "$output" = $'code=false\ninstall=false' ]
}

@test "on main, a new-branch push with no previous tip runs everything" {
  commit docs/src/content/docs/index.mdx
  GITHUB_REF="refs/heads/main" run --separate-stderr "$SCRIPT" "0000000000000000000000000000000000000000"
  [ "$output" = $'code=true\ninstall=true' ]
}

@test "on main, a force push runs everything" {
  commit docs/src/content/docs/index.mdx
  local before
  before="$(git rev-parse HEAD)"
  git reset --quiet --hard "$BASE"
  commit docs/src/content/docs/other.mdx

  GITHUB_REF="refs/heads/main" run --separate-stderr "$SCRIPT" "$before"
  [ "$output" = $'code=true\ninstall=true' ]
}

@test "missing origin/main runs everything" {
  commit docs/src/content/docs/index.mdx
  git update-ref -d refs/remotes/origin/main
  run --separate-stderr "$SCRIPT" ""
  [ "$output" = $'code=true\ninstall=true' ]
}

@test "empty diff runs everything" {
  run --separate-stderr "$SCRIPT" ""
  [ "$output" = $'code=true\ninstall=true' ]
}
