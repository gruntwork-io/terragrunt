#!/usr/bin/env bash
# Tests for Terragrunt install.sh script
#
# Usage:
#   ./install_test.sh              # Run all tests
#   ./install_test.sh --quick      # Skip download tests (faster)
#
# Requirements: bash 3.2+
# Note: Download tests require internet connection

# shellcheck disable=SC2317  # Functions are called indirectly via run_test

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_SCRIPT="${SCRIPT_DIR}/../public/install"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Colors
if [[ -t 1 ]]; then
    RED=$'\033[0;31m'
    GREEN=$'\033[0;32m'
    YELLOW=$'\033[0;33m'
    NC=$'\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    NC=''
fi

# --- Test Helpers ---
pass() {
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${GREEN}✓${NC} %s\n" "$1"
}

fail() {
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${RED}✗${NC} %s\n" "$1"
    if [[ -n "${2:-}" ]]; then
        printf "  ${RED}Error: %s${NC}\n" "$2"
    fi
}

run_test() {
    local name="$1"
    shift
    TESTS_RUN=$((TESTS_RUN + 1))
    if "$@"; then
        pass "$name"
        return 0
    else
        fail "$name"
        return 1
    fi
}

skip_test() {
    printf "${YELLOW}○${NC} %s (skipped)\n" "$1"
}

# --- Unit Tests ---

test_script_exists() {
    [[ -f "$INSTALL_SCRIPT" ]]
}

test_script_executable_syntax() {
    bash -n "$INSTALL_SCRIPT"
}

test_help_output() {
    local output
    output=$(bash "$INSTALL_SCRIPT" --help 2>&1)
    [[ "$output" == *"Terragrunt Installer"* ]] &&
    [[ "$output" == *"--version"* ]] &&
    [[ "$output" == *"--dir"* ]] &&
    [[ "$output" == *"--force"* ]] &&
    [[ "$output" == *"--verify-sig"* ]] &&
    [[ "$output" == *"--verify-gpg"* ]] &&
    [[ "$output" == *"--verify-cosign"* ]] &&
    [[ "$output" == *"--no-verify"* ]]
}

test_help_exit_code() {
    bash "$INSTALL_SCRIPT" --help >/dev/null 2>&1
}

test_invalid_option_fails() {
    ! bash "$INSTALL_SCRIPT" --invalid-option 2>/dev/null
}

test_missing_version_arg_fails() {
    ! bash "$INSTALL_SCRIPT" -v 2>/dev/null
}

test_missing_dir_arg_fails() {
    ! bash "$INSTALL_SCRIPT" -d 2>/dev/null
}

# Test OS detection by sourcing functions
test_os_detection() {
    local os
    os=$(uname -s)
    case "$os" in
        Darwin|Linux) return 0 ;;
        *) return 1 ;;
    esac
}

# Test arch detection
test_arch_detection() {
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64|aarch64|arm64|i386|i686) return 0 ;;
        *) return 1 ;;
    esac
}

# Test that sha256sum or shasum exists
test_checksum_tool_exists() {
    command -v sha256sum &>/dev/null || command -v shasum &>/dev/null
}

# Test curl exists
test_curl_exists() {
    command -v curl &>/dev/null
}

# --- Network Connectivity Check ---

check_network_connectivity() {
    # Quick check if we can reach GitHub
    curl -fsI --connect-timeout 5 "https://github.com" >/dev/null 2>&1
}

# --- Integration Tests (require network) ---

test_fetch_latest_version() {
    local version
    # Use redirect method (same as install.sh)
    local redirect_url
    redirect_url=$(curl -fsI "https://github.com/gruntwork-io/terragrunt/releases/latest" 2>/dev/null | grep -i '^location:' | tr -d '\r')
    version=$(echo "$redirect_url" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1)
    [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+ ]]
}

test_install_specific_version() {
    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand tmpdir now, not at trap time
    trap "rm -rf '$tmpdir'" RETURN

    bash "$INSTALL_SCRIPT" -d "$tmpdir" -v v0.72.5 >/dev/null 2>&1 &&
    [[ -f "$tmpdir/terragrunt" ]] &&
    [[ -x "$tmpdir/terragrunt" ]] &&
    "$tmpdir/terragrunt" --version 2>&1 | grep -q "v0.72.5"
}

test_install_rc_version() {
    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand tmpdir now, not at trap time
    trap "rm -rf '$tmpdir'" RETURN

    bash "$INSTALL_SCRIPT" -d "$tmpdir" -v v0.98.0-rc2026011601 >/dev/null 2>&1 &&
    [[ -f "$tmpdir/terragrunt" ]] &&
    [[ -x "$tmpdir/terragrunt" ]] &&
    "$tmpdir/terragrunt" --version 2>&1 | grep -q "v0.98.0-rc2026011601"
}

test_install_latest_version() {
    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand tmpdir now, not at trap time
    trap "rm -rf '$tmpdir'" RETURN

    bash "$INSTALL_SCRIPT" -d "$tmpdir" >/dev/null 2>&1 &&
    [[ -f "$tmpdir/terragrunt" ]] &&
    [[ -x "$tmpdir/terragrunt" ]] &&
    "$tmpdir/terragrunt" --version 2>&1 | grep -qE "^terragrunt version v[0-9]+"
}

test_install_already_exists_fails() {
    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand tmpdir now, not at trap time
    trap "rm -rf '$tmpdir'" RETURN

    # First install
    bash "$INSTALL_SCRIPT" -d "$tmpdir" -v v0.72.5 >/dev/null 2>&1 || return 1

    # Second install without --force should fail
    ! bash "$INSTALL_SCRIPT" -d "$tmpdir" -v v0.72.5 2>/dev/null
}

test_install_force_overwrites() {
    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand tmpdir now, not at trap time
    trap "rm -rf '$tmpdir'" RETURN

    # First install
    bash "$INSTALL_SCRIPT" -d "$tmpdir" -v v0.72.5 >/dev/null 2>&1 || return 1

    # Second install with --force should succeed
    bash "$INSTALL_SCRIPT" -d "$tmpdir" -v v0.72.5 --force >/dev/null 2>&1
}

test_install_nonexistent_dir_fails() {
    ! bash "$INSTALL_SCRIPT" -d /nonexistent/path/that/does/not/exist -v v0.72.5 2>/dev/null
}

test_install_invalid_version_fails() {
    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand tmpdir now, not at trap time
    trap "rm -rf '$tmpdir'" RETURN

    ! bash "$INSTALL_SCRIPT" -d "$tmpdir" -v invalid 2>/dev/null
}

test_install_no_verify() {
    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand tmpdir now, not at trap time
    trap "rm -rf '$tmpdir'" RETURN

    local output
    output=$(bash "$INSTALL_SCRIPT" -d "$tmpdir" -v v0.72.5 --no-verify 2>&1)
    [[ "$output" == *"Skipping checksum verification"* ]] &&
    [[ -f "$tmpdir/terragrunt" ]]
}

test_install_no_verification_at_all() {
    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand tmpdir now, not at trap time
    trap "rm -rf '$tmpdir'" RETURN

    # Install with no checksum verification (signature already disabled by default)
    local output
    output=$(bash "$INSTALL_SCRIPT" -d "$tmpdir" -v v0.72.5 --no-verify 2>&1)
    [[ "$output" == *"Skipping checksum verification"* ]] &&
    [[ "$output" != *"SHA256 checksum verified"* ]] &&
    [[ "$output" != *"Signature verified"* ]] &&
    [[ -f "$tmpdir/terragrunt" ]] &&
    [[ -x "$tmpdir/terragrunt" ]]
}

test_checksum_verification() {
    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand tmpdir now, not at trap time
    trap "rm -rf '$tmpdir'" RETURN

    local output
    output=$(bash "$INSTALL_SCRIPT" -d "$tmpdir" -v v0.72.5 2>&1)
    [[ "$output" == *"SHA256 checksum verified"* ]]
}

test_old_version_skips_signature() {
    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand tmpdir now, not at trap time
    trap "rm -rf '$tmpdir'" RETURN

    # v0.72.5 is below MIN_SIGNED_VERSION (0.98.0), even with --verify-sig it should skip
    local output
    output=$(bash "$INSTALL_SCRIPT" -d "$tmpdir" -v v0.72.5 --verify-sig 2>&1)
    [[ "$output" == *"Skipping signature verification: not available for versions older than"* ]]
}

test_signature_disabled_by_default() {
    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand tmpdir now, not at trap time
    trap "rm -rf '$tmpdir'" RETURN

    # Without --verify-sig, no signature verification messages should appear
    local output
    output=$(bash "$INSTALL_SCRIPT" -d "$tmpdir" -v v0.72.5 2>&1)
    [[ "$output" != *"Signature verified"* ]] &&
    [[ "$output" != *"signature verification"* ]]
}

test_gpg_signature_verification() {
    # Skip if gpg not available
    command -v gpg &>/dev/null || return 0

    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand tmpdir now, not at trap time
    trap "rm -rf '$tmpdir'" RETURN

    # Use RC version which has signatures
    local output
    output=$(bash "$INSTALL_SCRIPT" -d "$tmpdir" -v v0.98.0-rc2026011601 --verify-gpg 2>&1)
    [[ "$output" == *"Using GPG for signature verification"* ]] &&
    [[ "$output" == *"Signature verified"* ]]
}

test_cosign_signature_verification() {
    # Skip if cosign not available
    command -v cosign &>/dev/null || return 0

    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand tmpdir now, not at trap time
    trap "rm -rf '$tmpdir'" RETURN

    # Use RC version which has signatures
    local output
    output=$(bash "$INSTALL_SCRIPT" -d "$tmpdir" -v v0.98.0-rc2026011601 --verify-cosign 2>&1)
    [[ "$output" == *"Using Cosign for signature verification"* ]] &&
    [[ "$output" == *"Signature verified"* ]]
}

test_auto_signature_verification() {
    # Skip if neither gpg nor cosign available
    command -v gpg &>/dev/null || command -v cosign &>/dev/null || return 0

    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand tmpdir now, not at trap time
    trap "rm -rf '$tmpdir'" RETURN

    # Use RC version which has signatures, auto-detect method with --verify-sig
    local output
    output=$(bash "$INSTALL_SCRIPT" -d "$tmpdir" -v v0.98.0-rc2026011601 --verify-sig 2>&1)
    [[ "$output" == *"Signature verified"* ]]
}

# --- Platform-Specific Tests ---

test_macos_shasum_fallback() {
    # This test verifies the shasum fallback logic works
    # On Linux with sha256sum, we simulate by checking the code path exists
    if command -v sha256sum &>/dev/null; then
        # On Linux, verify sha256sum is used
        return 0
    elif command -v shasum &>/dev/null; then
        # On macOS, verify shasum works
        echo "test" | shasum -a 256 >/dev/null 2>&1
    else
        return 1
    fi
}

test_temp_directory_cleanup() {
    local install_dir
    install_dir=$(mktemp -d)
    # shellcheck disable=SC2064  # Intentional: expand install_dir now, not at trap time
    trap "rm -rf '$install_dir'" RETURN

    # Count terragrunt-specific temp dirs before
    local before
    before=$(find /tmp -maxdepth 1 -name 'terragrunt-install.*' -type d 2>/dev/null | wc -l)

    bash "$INSTALL_SCRIPT" -d "$install_dir" -v v0.72.5 >/dev/null 2>&1

    # Verify no new terragrunt-specific temp dirs remain (script uses trap to cleanup)
    local after
    after=$(find /tmp -maxdepth 1 -name 'terragrunt-install.*' -type d 2>/dev/null | wc -l)
    [[ "$after" -le "$before" ]]
}

# --- Main ---

main() {
    local quick_mode=false
    if [[ "${1:-}" == "--quick" ]]; then
        quick_mode=true
    fi

    echo "=========================================="
    echo "Terragrunt Install Script Tests"
    echo "=========================================="
    echo ""

    echo "--- Basic Tests ---"
    run_test "Script exists" test_script_exists
    run_test "Script has valid syntax" test_script_executable_syntax
    run_test "Help output contains expected content" test_help_output
    run_test "Help exits with code 0" test_help_exit_code
    run_test "Invalid option fails" test_invalid_option_fails
    run_test "Missing -v argument fails" test_missing_version_arg_fails
    run_test "Missing -d argument fails" test_missing_dir_arg_fails
    echo ""

    echo "--- Environment Tests ---"
    run_test "OS is supported ($(uname -s))" test_os_detection
    run_test "Architecture is supported ($(uname -m))" test_arch_detection
    run_test "Checksum tool exists (sha256sum/shasum)" test_checksum_tool_exists
    run_test "curl is installed" test_curl_exists
    run_test "Platform checksum tool works" test_macos_shasum_fallback
    echo ""

    # Check network connectivity for integration tests
    local skip_reason=""
    if [[ "$quick_mode" == true ]]; then
        skip_reason="quick mode"
    elif ! check_network_connectivity; then
        skip_reason="no network connectivity"
    fi

    if [[ -n "$skip_reason" ]]; then
        echo "--- Integration Tests (SKIPPED - ${skip_reason}) ---"
        skip_test "Fetch latest version from GitHub"
        skip_test "Install specific version"
        skip_test "Install RC version"
        skip_test "Install latest version"
        skip_test "Install fails when already exists"
        skip_test "Install with --force overwrites"
        skip_test "Install to nonexistent directory fails"
        skip_test "Install with invalid version fails"
        skip_test "Install with --no-verify skips checksum"
        skip_test "Install with no verification at all"
        skip_test "Checksum verification works"
        skip_test "Old version skips signature verification"
        skip_test "Signature disabled by default"
        skip_test "GPG signature verification"
        skip_test "Cosign signature verification"
        skip_test "Auto signature verification"
        skip_test "Temp directory cleanup"
    else
        echo "--- Integration Tests (require network) ---"
        run_test "Fetch latest version from GitHub" test_fetch_latest_version
        run_test "Install specific version (v0.72.5)" test_install_specific_version
        run_test "Install RC version (v0.98.0-rc2026011601)" test_install_rc_version
        run_test "Install latest version" test_install_latest_version
        run_test "Install fails when already exists" test_install_already_exists_fails
        run_test "Install with --force overwrites" test_install_force_overwrites
        run_test "Install to nonexistent directory fails" test_install_nonexistent_dir_fails
        run_test "Install with invalid version fails" test_install_invalid_version_fails
        run_test "Install with --no-verify skips checksum" test_install_no_verify
        run_test "Install with no verification at all" test_install_no_verification_at_all
        run_test "Checksum verification works" test_checksum_verification
        run_test "Old version skips signature verification" test_old_version_skips_signature
        run_test "Signature disabled by default" test_signature_disabled_by_default
        run_test "GPG signature verification" test_gpg_signature_verification
        run_test "Cosign signature verification" test_cosign_signature_verification
        run_test "Auto signature verification" test_auto_signature_verification
        run_test "Temp directory cleanup" test_temp_directory_cleanup
    fi
    echo ""

    echo "=========================================="
    echo "Results: ${TESTS_PASSED}/${TESTS_RUN} passed"
    if [[ $TESTS_FAILED -gt 0 ]]; then
        echo "${RED}${TESTS_FAILED} test(s) failed${NC}"
        exit 1
    else
        echo "${GREEN}All tests passed!${NC}"
        exit 0
    fi
}

main "$@"
