#!/usr/bin/env bash

set -euo pipefail

# Reclaim runner disk space before suites that pull providers and images, and
# report what the reclaim actually bought.

# -P keeps each filesystem on one line, so a long device name cannot shift the
# field the available figure is read from. -k fixes the unit at 1K blocks so the
# reclaimed figure is arithmetic rather than parsed text.
function available_kb {
	df -Pk / | awk 'NR == 2 { print $4 }'
}

function human {
	awk -v kb="$1" 'BEGIN { printf "%.2f GiB", kb / 1024 / 1024 }'
}

before="$(available_kb)"
echo "Free space before: $(human "$before")"

sudo rm -rf /usr/share/dotnet /usr/local/lib/android /opt/ghc /opt/hostedtoolcache/CodeQL
sudo docker image prune --all --force
sudo docker builder prune -a

after="$(available_kb)"
echo "Free space after:  $(human "$after")"
echo "Reclaimed:         $(human "$((after - before))")"

df -h
