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
  init|plan)
    ;;
  output)
    if [[ "${2:-}" == "-json" ]]; then
      printf '{}\n'
    else
      printf 'unexpected fake Terraform output arguments: %s\n' "$*" >&2
      exit 1
    fi
    ;;
  *)
    printf 'unexpected fake Terraform command: %s\n' "$cmd" >&2
    exit 1
    ;;
esac
