# Shared setup for the update-codified-remote-deps tests.

# common_setup: runs the test from its own tmpdir, where the scripts read their
# repo-relative pin files, and puts stubbed commands first on PATH.
common_setup() {
  cd "$BATS_TEST_TMPDIR"

  export GITHUB_OUTPUT="$BATS_TEST_TMPDIR/github-output"
  export STUB_LOG="$BATS_TEST_TMPDIR/stub.log"

  STUB_DIR="$BATS_TEST_TMPDIR/stubs"
  mkdir -p "$STUB_DIR"
  export PATH="$STUB_DIR:$PATH"
}

# install_stub <command>
install_stub() {
  cp "$BATS_TEST_DIRNAME/helpers/command-stub.sh" "$STUB_DIR/$1"
  chmod +x "$STUB_DIR/$1"
}

# only_tools_path <tool>...: echoes a PATH holding bash, the commands every
# script needs to source lib.sh, and the named tools.
only_tools_path() {
  local -r dir="$BATS_TEST_TMPDIR/only-tools"
  local tool

  mkdir -p "$dir"
  for tool in bash dirname basename "$@"; do
    ln -sf "$(command -v "$tool")" "$dir/$tool"
  done

  printf '%s' "$dir"
}
