#!/bin/bash
# Terragrunt Installer
#
# Supported platforms: Linux, macOS (Darwin)
# Supported architectures: x86_64 (amd64), aarch64/arm64, i386/i686 (386)
# Requirements: bash 3.2+, curl, sha256sum or shasum
#
# Usage:
#   curl -sL https://terragrunt.gruntwork.io/scripts/install.sh | bash
#   curl -sL https://terragrunt.gruntwork.io/scripts/install.sh | bash -s -- -v v0.72.5
#   curl -sL https://terragrunt.gruntwork.io/scripts/install.sh | bash -s -- -d ~/bin
#
# Options:
#   -v, --version VERSION    Install specific version (default: latest)
#   -d, --dir PATH           Installation directory (default: /usr/local/bin)
#   -f, --force              Overwrite existing installation
#   --verify-gpg             Verify GPG signature (requires gpg)
#   --no-verify              Skip checksum verification
#   -h, --help               Show this help message
#
# Environment:
#   TERRAGRUNT_VERSION       Override version (same as -v)
#   TERRAGRUNT_INSTALL_DIR   Override install directory (same as -d)

set -euo pipefail

# --- Constants ---
readonly GITHUB_REPO="gruntwork-io/terragrunt"
readonly GPG_KEY_URL="https://gruntwork.io/.well-known/pgp-key.txt"
readonly DEFAULT_INSTALL_DIR="/usr/local/bin"
readonly BINARY_NAME="terragrunt"

# --- Colors (if terminal) ---
# Use $'...' syntax for reliable escape sequence interpretation on macOS/Linux
if [[ -t 1 ]]; then
    readonly RED=$'\033[0;31m'
    readonly GREEN=$'\033[0;32m'
    readonly YELLOW=$'\033[0;33m'
    readonly BLUE=$'\033[0;34m'
    readonly NC=$'\033[0m' # No Color
else
    readonly RED=''
    readonly GREEN=''
    readonly YELLOW=''
    readonly BLUE=''
    readonly NC=''
fi

# --- Helper Functions ---
abort() {
    printf "${RED}Error: %s${NC}\n" "$1" >&2
    exit 1
}

info() {
    printf "${BLUE}==> ${NC}%s\n" "$1"
}

warn() {
    printf "${YELLOW}Warning: %s${NC}\n" "$1" >&2
}

success() {
    printf "${GREEN}==> %s${NC}\n" "$1"
}

usage() {
    cat <<EOF
Terragrunt Installer

Usage:
  curl -sL https://terragrunt.gruntwork.io/scripts/install.sh | bash
  curl -sL https://terragrunt.gruntwork.io/scripts/install.sh | bash -s -- [OPTIONS]

Options:
  -v, --version VERSION    Install specific version (default: latest)
  -d, --dir PATH           Installation directory (default: /usr/local/bin)
  -f, --force              Overwrite existing installation
  --verify-gpg             Verify GPG signature (requires gpg)
  --no-verify              Skip checksum verification
  -h, --help               Show this help message

Examples:
  # Install latest version
  curl -sL https://terragrunt.gruntwork.io/scripts/install.sh | bash

  # Install specific version
  curl -sL https://terragrunt.gruntwork.io/scripts/install.sh | bash -s -- -v v0.72.5

  # Install to custom directory
  curl -sL https://terragrunt.gruntwork.io/scripts/install.sh | bash -s -- -d ~/bin

  # Install with GPG verification
  curl -sL https://terragrunt.gruntwork.io/scripts/install.sh | bash -s -- --verify-gpg
EOF
}

# --- OS/Arch Detection ---
detect_os() {
    local os
    os="$(uname -s)"
    case "$os" in
        Darwin) echo "darwin" ;;
        Linux)  echo "linux" ;;
        MINGW*|MSYS*|CYGWIN*)
            abort "Windows detected. Please use PowerShell or install via Chocolatey:
  choco install terragrunt

Or download manually from: https://github.com/gruntwork-io/terragrunt/releases"
            ;;
        *)
            abort "Unsupported operating system: $os
Supported: Linux, macOS (Darwin)"
            ;;
    esac
}

detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        i386|i686)     echo "386" ;;
        *)
            abort "Unsupported architecture: $arch
Supported: x86_64 (amd64), aarch64 (arm64), i386/i686 (386)"
            ;;
    esac
}

# --- Version Resolution ---
get_latest_version() {
    local version
    if ! version=$(curl -sL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null | grep -o '"tag_name": "[^"]*' | cut -d'"' -f4); then
        abort "Failed to fetch latest version from GitHub API"
    fi
    if [[ -z "$version" ]]; then
        abort "Could not determine latest version. Check your internet connection or specify a version with -v"
    fi
    echo "$version"
}

validate_version() {
    local version="$1"
    # Accept versions with or without 'v' prefix
    if [[ ! "$version" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+ ]]; then
        abort "Invalid version format: $version
Expected format: v0.72.5 or 0.72.5"
    fi
    # Ensure version has 'v' prefix
    if [[ ! "$version" =~ ^v ]]; then
        version="v$version"
    fi
    echo "$version"
}

# --- Download Functions ---
download_file() {
    local url="$1"
    local output="$2"
    local description="$3"

    info "Downloading $description..."
    if ! curl -sL --fail "$url" -o "$output" 2>/dev/null; then
        abort "Failed to download $description from: $url"
    fi
}

download_binary() {
    local version="$1"
    local binary_name="$2"
    local output_dir="$3"

    local url="https://github.com/${GITHUB_REPO}/releases/download/${version}/${binary_name}"
    download_file "$url" "${output_dir}/${binary_name}" "Terragrunt ${version}"
}

download_checksums() {
    local version="$1"
    local output_dir="$2"

    local url="https://github.com/${GITHUB_REPO}/releases/download/${version}/SHA256SUMS"
    download_file "$url" "${output_dir}/SHA256SUMS" "checksums"
}

download_gpg_signature() {
    local version="$1"
    local output_dir="$2"

    local url="https://github.com/${GITHUB_REPO}/releases/download/${version}/SHA256SUMS.gpgsig"
    download_file "$url" "${output_dir}/SHA256SUMS.gpgsig" "GPG signature"
}

# --- Verification Functions ---
verify_sha256() {
    local binary_path="$1"
    local checksums_path="$2"
    local binary_name="$3"

    info "Verifying SHA256 checksum..."

    local actual_checksum
    if command -v sha256sum &>/dev/null; then
        actual_checksum=$(sha256sum "$binary_path" | awk '{print $1}')
    elif command -v shasum &>/dev/null; then
        actual_checksum=$(shasum -a 256 "$binary_path" | awk '{print $1}')
    else
        abort "Neither sha256sum nor shasum found. Cannot verify checksum."
    fi

    local expected_checksum
    expected_checksum=$(awk -v bin="$binary_name" '$2 == bin {print $1; exit}' "$checksums_path")

    if [[ -z "$expected_checksum" ]]; then
        abort "Could not find checksum for $binary_name in SHA256SUMS file"
    fi

    if [[ "$actual_checksum" != "$expected_checksum" ]]; then
        abort "Checksum verification failed!
Expected: $expected_checksum
Got:      $actual_checksum

The downloaded file may be corrupted or tampered with."
    fi
}

verify_gpg() {
    local checksums_path="$1"
    local signature_path="$2"

    if ! command -v gpg &>/dev/null; then
        abort "GPG verification requested but gpg is not installed.
Install gpg or remove the --verify-gpg flag."
    fi

    info "Importing Gruntwork GPG key..."
    if ! curl -sL "$GPG_KEY_URL" | gpg --import 2>/dev/null; then
        warn "Failed to import GPG key. Attempting verification with existing keyring..."
    fi

    info "Verifying GPG signature..."
    if ! gpg --verify "$signature_path" "$checksums_path" 2>/dev/null; then
        abort "GPG signature verification failed!
The checksums file signature is invalid."
    fi
}

# --- Installation ---
install_binary() {
    local binary_path="$1"
    local install_dir="$2"
    local force="$3"

    local target_path="${install_dir}/${BINARY_NAME}"

    # Check if already exists
    if [[ -f "$target_path" && "$force" != "true" ]]; then
        local existing_version
        existing_version=$("$target_path" --version 2>/dev/null | head -n 1 || echo "unknown")
        abort "Terragrunt already installed at $target_path ($existing_version)
Use --force to overwrite, or remove the existing installation first."
    fi

    # Check if install directory exists
    if [[ ! -d "$install_dir" ]]; then
        abort "Installation directory does not exist: $install_dir
Create it first or specify a different directory with -d"
    fi

    # Check write permissions
    if [[ ! -w "$install_dir" ]]; then
        abort "Cannot write to $install_dir
Try running with sudo or specify a different directory with -d"
    fi

    info "Installing to ${target_path}..."
    chmod +x "$binary_path"
    mv "$binary_path" "$target_path"
}

# --- Argument Parsing ---
parse_args() {
    # Set defaults from environment or hardcoded values
    VERSION="${TERRAGRUNT_VERSION:-}"
    INSTALL_DIR="${TERRAGRUNT_INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
    VERIFY_SHA=true
    VERIFY_GPG=false
    FORCE=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            -v|--version)
                [[ -z "${2:-}" ]] && abort "Option $1 requires a version argument"
                VERSION="$2"
                shift 2
                ;;
            -d|--dir)
                [[ -z "${2:-}" ]] && abort "Option $1 requires a directory argument"
                INSTALL_DIR="$2"
                shift 2
                ;;
            -f|--force)
                FORCE=true
                shift
                ;;
            --verify-gpg)
                VERIFY_GPG=true
                shift
                ;;
            --no-verify)
                VERIFY_SHA=false
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            -*)
                abort "Unknown option: $1
Use -h or --help for usage information"
                ;;
            *)
                abort "Unexpected argument: $1
Use -h or --help for usage information"
                ;;
        esac
    done
}

# --- Dependency Check ---
check_dependencies() {
    if ! command -v curl &>/dev/null; then
        abort "curl is required but not installed.
Please install curl and try again."
    fi
}

# --- Main ---
main() {
    parse_args "$@"

    # Check dependencies
    check_dependencies

    # Detect platform
    local os arch version binary_name
    os=$(detect_os)
    arch=$(detect_arch)

    # Resolve version
    if [[ -z "$VERSION" ]]; then
        info "Fetching latest version..."
        version=$(get_latest_version)
    else
        version=$(validate_version "$VERSION")
    fi

    binary_name="terragrunt_${os}_${arch}"

    info "Installing Terragrunt ${version} for ${os}/${arch}"

    # Create temp directory
    local tmpdir
    tmpdir=$(mktemp -d)
    trap 'rm -rf "${tmpdir:-}"' EXIT

    # Download files
    download_binary "$version" "$binary_name" "$tmpdir"
    download_checksums "$version" "$tmpdir"

    # Verify checksum
    if [[ "$VERIFY_SHA" == true ]]; then
        verify_sha256 "$tmpdir/$binary_name" "$tmpdir/SHA256SUMS" "$binary_name"
        success "SHA256 checksum verified"
    else
        warn "Skipping checksum verification (--no-verify specified)"
    fi

    # Verify GPG signature (optional)
    if [[ "$VERIFY_GPG" == true ]]; then
        download_gpg_signature "$version" "$tmpdir"
        verify_gpg "$tmpdir/SHA256SUMS" "$tmpdir/SHA256SUMS.gpgsig"
        success "GPG signature verified"
    fi

    # Install
    install_binary "$tmpdir/$binary_name" "$INSTALL_DIR" "$FORCE"

    success "Terragrunt ${version} installed successfully!"
    echo ""
    echo "Run 'terragrunt --version' to verify the installation."
    echo "For documentation, visit: https://terragrunt.gruntwork.io/docs"
}

main "$@"
