#!/bin/bash

set -e

# Script to verify that binaries are statically linked
# Usage: verify-static-binary.sh <binary-path> <os> <arch>

function verify_linux_binary() {
  local -r binary="$1"

  echo "Verifying Linux binary is statically linked..."
  file "$binary"

  # Check for static linking
  file "$binary" | grep -q "statically linked"
  echo "✓ Linux binary is statically linked"

  # Verify no dynamic dependencies
  ldd "$binary" 2>&1 | grep -q "not a dynamic executable"
  echo "✓ Linux binary has no dynamic dependencies"
}

function verify_darwin_binary() {
  local -r binary="$1"

  echo "Verifying macOS binary..."
  file "$binary"

  # Check Mach-O format
  file "$binary" | grep -q "Mach-O.*executable"
  echo "✓ macOS binary is Mach-O executable"

  # Check for minimal dynamic dependencies
  # macOS binaries should only link to system libraries
  local unexpected_deps
  unexpected_deps=$(otool -L "$binary" | grep -v "$binary:" | grep -v "/usr/lib/libSystem" | grep -v "/usr/lib/libresolv" | grep "/" || true)

  [ -z "$unexpected_deps" ]
  echo "✓ macOS binary has minimal system dependencies"
}

function verify_windows_binary() {
  local -r binary="$1"

  echo "Verifying Windows binary..."
  file "$binary"

  # Check PE format
  file "$binary" | grep -q "PE32.*executable.*Windows"
  echo "✓ Windows binary is PE32 executable"

  # Check for non-standard DLLs (CGO would add mingw dependencies)
  local unexpected_dlls
  unexpected_dlls=$(objdump -p "$binary" 2>/dev/null | grep -i "DLL Name" | grep -v -i "KERNEL32.dll\|msvcrt.dll\|WS2_32.dll\|ADVAPI32.dll\|SHELL32.dll\|ole32.dll" || true)

  [ -z "$unexpected_dlls" ]
  echo "✓ Windows binary has standard system DLL dependencies only"
}

function main() {
  local -r binary="$1"
  local -r os="$2"
  local -r arch="$3"

  [ -n "$binary" ] || { echo "ERROR: binary path is required"; exit 1; }
  [ -n "$os" ] || { echo "ERROR: os is required"; exit 1; }
  [ -n "$arch" ] || { echo "ERROR: arch is required"; exit 1; }

  [ -f "$binary" ] || { echo "ERROR: Binary $binary does not exist"; exit 1; }

  echo "Verifying static linking for $binary ($os/$arch)..."

  case "$os" in
    linux)
      verify_linux_binary "$binary"
      ;;
    darwin)
      verify_darwin_binary "$binary"
      ;;
    windows)
      verify_windows_binary "$binary"
      ;;
    *)
      echo "ERROR: Unsupported OS: $os"
      exit 1
      ;;
  esac

  echo "Static linking verification passed for $os/$arch!"
}

main "$@"
