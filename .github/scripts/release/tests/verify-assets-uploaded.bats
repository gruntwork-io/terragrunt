#!/usr/bin/env bats

setup() {
  SCRIPT="${BATS_TEST_DIRNAME}/../verify-assets-uploaded.sh"

  # The script reads the release config by a path relative to the repo root.
  cd "${BATS_TEST_DIRNAME}/../../../.."

  STUB_DIR="${BATS_TEST_TMPDIR}/stubs"
  mkdir -p "$STUB_DIR"
  cp "${BATS_TEST_DIRNAME}/helpers/gh-release-stub.sh" "${STUB_DIR}/gh"
  printf '#!/usr/bin/env bash\n' >"${STUB_DIR}/sleep"
  chmod +x "${STUB_DIR}/gh" "${STUB_DIR}/sleep"
  export PATH="${STUB_DIR}:${PATH}"

  BIN_DIR="${BATS_TEST_TMPDIR}/bin"
  export GH_STUB_RELEASE_DIR="${BATS_TEST_TMPDIR}/release"
  export GH_STUB_UPLOAD_FAILURES="${BATS_TEST_TMPDIR}/upload-failures"
  export GH_STUB_LOG="${BATS_TEST_TMPDIR}/gh.log"
  mkdir -p "$BIN_DIR" "$GH_STUB_RELEASE_DIR"
  touch "$GH_STUB_LOG"

  source "${BATS_TEST_DIRNAME}/../lib-release-config.sh"
  EXPECTED_COUNT="$(get_total_file_count)"

  local file
  while IFS= read -r file; do
    echo "content of $file" >"${BIN_DIR}/${file}"
    cp "${BIN_DIR}/${file}" "${GH_STUB_RELEASE_DIR}/${file}"
  done < <(get_all_expected_files)

  export GH_TOKEN=fake
  export VERSION=v1.0.0
}

@test "passes without uploading when every asset is present" {
  run "$SCRIPT" "$BIN_DIR"
  [ "$status" -eq 0 ]
  [[ "$output" == *"Release assets: ${EXPECTED_COUNT}"* ]]
  [[ "$output" != *"Warning"* ]]
  ! grep -q "^release upload" "$GH_STUB_LOG"
}

@test "uploads missing assets and counts them" {
  rm "${GH_STUB_RELEASE_DIR}/SHA256SUMS.pem" "${GH_STUB_RELEASE_DIR}/terragrunt_linux_amd64.zip"

  run "$SCRIPT" "$BIN_DIR"
  [ "$status" -eq 0 ]
  [ -f "${GH_STUB_RELEASE_DIR}/SHA256SUMS.pem" ]
  [ -f "${GH_STUB_RELEASE_DIR}/terragrunt_linux_amd64.zip" ]
  [[ "$output" == *"Release assets: ${EXPECTED_COUNT}"* ]]
  [[ "$output" != *"Warning"* ]]
}

@test "retries a failed upload until it succeeds" {
  rm "${GH_STUB_RELEASE_DIR}/SHA256SUMS.pem"
  echo 3 >"$GH_STUB_UPLOAD_FAILURES"

  run "$SCRIPT" "$BIN_DIR"
  [ "$status" -eq 0 ]
  [ -f "${GH_STUB_RELEASE_DIR}/SHA256SUMS.pem" ]
  [ "$(grep -c "^release upload" "$GH_STUB_LOG")" -eq 4 ]
}

@test "passes --clobber to uploads when CLOBBER is true" {
  rm "${GH_STUB_RELEASE_DIR}/SHA256SUMS.pem"
  export CLOBBER=true

  run "$SCRIPT" "$BIN_DIR"
  [ "$status" -eq 0 ]
  grep -q "^release upload v1.0.0 ${BIN_DIR}/SHA256SUMS.pem --clobber$" "$GH_STUB_LOG"
}

@test "fails when an upload never succeeds" {
  rm "${GH_STUB_RELEASE_DIR}/SHA256SUMS.pem"
  echo 100 >"$GH_STUB_UPLOAD_FAILURES"

  run "$SCRIPT" "$BIN_DIR"
  [ "$status" -ne 0 ]
  [[ "$output" == *"Failed to upload SHA256SUMS.pem after 10 attempts"* ]]
}

@test "fails when a missing asset has no local file" {
  rm "${GH_STUB_RELEASE_DIR}/SHA256SUMS.pem" "${BIN_DIR}/SHA256SUMS.pem"

  run "$SCRIPT" "$BIN_DIR"
  [ "$status" -ne 0 ]
  [[ "$output" == *"SHA256SUMS.pem not found locally"* ]]
}

@test "fails when an asset is still missing after its upload reports success" {
  rm "${GH_STUB_RELEASE_DIR}/SHA256SUMS.pem"
  export GH_STUB_DROP_UPLOADS=true

  run "$SCRIPT" "$BIN_DIR"
  [ "$status" -ne 0 ]
  [[ "$output" == *"missing 1 of ${EXPECTED_COUNT} expected files"* ]]
  [[ "$output" == *"SHA256SUMS.pem"* ]]
}

@test "fails when the downloaded asset differs from the local file" {
  echo "stale" >"${GH_STUB_RELEASE_DIR}/SHA256SUMS"

  run "$SCRIPT" "$BIN_DIR"
  [ "$status" -ne 0 ]
  [[ "$output" == *"Downloaded SHA256SUMS does not match"* ]]
}

@test "fails when the release cannot be listed" {
  export GH_STUB_VIEW_FAILS=true

  run "$SCRIPT" "$BIN_DIR"
  [ "$status" -ne 0 ]
  [[ "$output" != *"All required assets verified"* ]]
  ! grep -q "^release upload" "$GH_STUB_LOG"
}

@test "fails when VERSION missing" {
  unset VERSION
  run "$SCRIPT" "$BIN_DIR"
  [ "$status" -ne 0 ]
  [[ "$output" == *"VERSION"* ]]
}

@test "fails when GH_TOKEN missing" {
  unset GH_TOKEN
  run "$SCRIPT" "$BIN_DIR"
  [ "$status" -ne 0 ]
  [[ "$output" == *"GH_TOKEN"* ]]
}
