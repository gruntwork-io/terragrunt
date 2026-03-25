#!/usr/bin/env bash
#
# Mount tmpfs (Linux) or RAM disk (macOS) for faster CI builds.
#
# Usage: mount-tmpfs.sh [OPTIONS] [EXTRA_PATH:SIZE ...]
#   --go-build-only   Only mount go-build cache (minimal mode)
#   EXTRA_PATH:SIZE   Additional cache dirs under ~/.cache to mount
#                     e.g. terragrunt:4G golangci-lint:2G

set -euo pipefail

GO_BUILD_ONLY=false
EXTRAS=()

for arg in "$@"; do
	case "$arg" in
	--go-build-only) GO_BUILD_ONLY=true ;;
	*) EXTRAS+=("$arg") ;;
	esac
done

case "$(uname -s)" in
Linux)
	if [[ "$GO_BUILD_ONLY" == "true" ]]; then
		mkdir -p /home/runner/.cache/go-build
		sudo mount -t tmpfs -o size=4G tmpfs /home/runner/.cache/go-build
	else
		sudo mount -t tmpfs -o size=12G tmpfs /tmp
		mkdir -p /home/runner/go
		sudo mount -t tmpfs -o size=12G tmpfs /home/runner/go
		mkdir -p /home/runner/.cache/go-build
		sudo mount -t tmpfs -o size=4G tmpfs /home/runner/.cache/go-build
	fi

	for extra in "${EXTRAS[@]}"; do
		path="/home/runner/.cache/${extra%%:*}"
		size="${extra##*:}"
		mkdir -p "$path"
		sudo mount -t tmpfs -o size="$size" tmpfs "$path"
	done
	;;
Darwin)
	RAMDISK=$(hdiutil attach -nomount ram://8388608 | tr -d '[:space:]') || {
		echo "Failed to create RAM disk"
		exit 1
	}
	# Wait for disk device to be registered in kernel
	for _ in $(seq 1 10); do
		diskutil info "$RAMDISK" >/dev/null 2>&1 && break
		sleep 1
	done
	diskutil erasevolume HFS+ RAMDisk "$RAMDISK"
	mkdir -p /Volumes/RAMDisk/go-build
	rm -rf ~/Library/Caches/go-build
	ln -sf /Volumes/RAMDisk/go-build ~/Library/Caches/go-build
	;;
*)
	echo "Unsupported OS: $(uname -s)" >&2
	exit 1
	;;
esac
