#!/bin/bash

set -euo pipefail

# Script to verify that binaries are statically linked
# Usage: verify-static-binary.sh <binary-path> <os> <arch>

die() {
  echo "ERROR: $*" >&2
  exit 1
}

require_arg() {
  local -r value="$1"
  local -r name="$2"

  [[ -n "$value" ]] || die "$name is required"
}

expect_empty() {
  local -r value="$1"
  local -r message="$2"

  [[ -z "$value" ]] || {
    echo "$message" >&2
    echo "$value" >&2
    exit 1
  }
}

verify_linux_binary() {
  local -r binary="$1"
  local -r file_info="$2"

  echo "Verifying Linux binary is statically linked..."
  echo "$file_info"

  grep -q "statically linked" <<<"$file_info"
  echo "Linux binary is statically linked"

  # Verify with ldd - it should either say "not a dynamic executable" or "statically linked"
  local ldd_output
  ldd_output=$(ldd "$binary" 2>&1 || true)

  grep -qE "not a dynamic executable|statically linked" <<<"$ldd_output"
  echo "Linux binary has no dynamic dependencies"
}

verify_darwin_binary() {
  local -r binary="$1"
  local -r file_info="$2"

  echo "Verifying macOS binary..."
  echo "$file_info"

  grep -q "Mach-O.*executable" <<<"$file_info"
  echo "macOS binary is Mach-O executable"
}

verify_windows_binary() {
  local -r binary="$1"
  local -r file_info="$2"

  echo "Verifying Windows binary..."
  echo "$file_info"

  grep -q "PE32.*executable.*Windows" <<<"$file_info"
  echo "Windows binary is PE32 executable"

  local unexpected_dlls
  unexpected_dlls=$(
    objdump -p "$binary" 2>/dev/null |
      grep -i "DLL Name" |
      grep -vi "KERNEL32.dll\|msvcrt.dll\|WS2_32.dll\|ADVAPI32.dll\|SHELL32.dll\|ole32.dll" || true
  )

  expect_empty "$unexpected_dlls" "Windows binary links to unexpected DLLs:"
  echo "Windows binary has standard system DLL dependencies only"
}

main() {
  local -r binary="${1:-}"
  local -r os="${2:-}"
  local -r arch="${3:-}"

  require_arg "$binary" "binary path"
  require_arg "$os" "os"
  require_arg "$arch" "arch"

  [[ -f "$binary" ]] || die "Binary $binary does not exist"

  echo "Verifying static linking for $binary ($os/$arch)..."
  local file_info
  file_info=$(file "$binary")

  case "$os" in
    linux)
      verify_linux_binary "$binary" "$file_info"
      ;;
    darwin)
      verify_darwin_binary "$binary" "$file_info"
      ;;
    windows)
      verify_windows_binary "$binary" "$file_info"
      ;;
    *)
      die "Unsupported OS: $os"
      ;;
  esac

  echo "Static linking verification passed for $os/$arch!"
}

main "$@"
