#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
counter_file="${script_dir}/no_cache_counter.txt"

count=0
if [[ -f "${counter_file}" ]]; then
  if ! read -r count <"${counter_file}"; then
    count=0
  fi
fi

count=$((count + 1))
printf "%s" "${count}" >"${counter_file}"

echo "no-cache-value-${count}"
