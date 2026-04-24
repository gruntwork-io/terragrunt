#!/usr/bin/env bash
set -euo pipefail

cmd="${1:-}"

case "$cmd" in
  version|--version|-version)
    if [[ "${2:-}" == "-json" ]]; then
      printf '{"terraform_version":"1.14.8"}\n'
    else
      printf 'Terraform v1.14.8\n'
    fi
    ;;
  output)
    if [[ "${2:-}" == "-json" ]]; then
      printf '{}\n'
    fi
    ;;
  *)
    ;;
esac
