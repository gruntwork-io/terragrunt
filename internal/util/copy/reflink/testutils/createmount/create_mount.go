// Package createmount provides utilities for creating and mounting disk images for testing
package createmount

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/util/copy/reflink/testutils/ts"
)

// calculate imageSizeMB for the image (e.g., 500MB)
const imageSizeMB = 512

// MountDiskImageMacOS creates a disk image with the specified filesystem type,
// mounts it, and registers cleanup on the provided testing.TB.
func MountDiskImageMacOS(tb testing.TB, mountpoint, fsType string) {
	tb.Helper()

	if runtime.GOOS != "darwin" {
		tb.Fatalf("this only works on macOS")
	}

	// Create the disk image file path in the temp directory managed by TB
	imagePath := filepath.Join(tb.TempDir(), "test_image.sparseimage")

	// Create the disk image file
	cmd := exec.CommandContext(tb.Context(), "hdiutil", "create", "-size", strconv.Itoa(imageSizeMB)+"m", "-fs", fsType, "-volname", "TestVolume", "-type", "SPARSE", "-quiet", imagePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("failed to create disk image: %v, output: %s", err, string(out))
	}

	// Mount the disk image to the provided location
	mountCmd := exec.CommandContext(tb.Context(), "hdiutil", "attach", "-mountPoint", mountpoint, imagePath)
	if err := mountCmd.Run(); err != nil {
		tb.Fatalf("failed to mount disk image: %v", err)
	}

	// Register cleanup to unmount the disk after the test
	// this may not be necessary since it seemse deleting the image and mount location does this automatically
	tb.Cleanup(func() {
		const (
			maxRetries   = 5
			initialDelay = 100 * time.Millisecond
		)

		for attempt := range ts.Backoff(maxRetries, initialDelay, time.Second) {
			unmountCmd := exec.CommandContext(tb.Context(), "hdiutil", "detach", mountpoint)
			if out, err := unmountCmd.CombinedOutput(); err != nil {
				if slices.Contains(retryableDetachExitCodes, unmountCmd.ProcessState.ExitCode()) {
					tb.Logf("warning: failed to unmount disk image (attempt %d): %v, output: %s", attempt+1, err, string(out))
					tb.Logf("if this was \"No such file or directory\", it may be safe to ignore as the test cleanup may have just deleted the file and mount point")

					continue
				}

				tb.Logf("failed to unmount disk image: %v, output: %s", err, string(out))

				break
			} else {
				break
			}
		}
	})
}

var retryableDetachExitCodes = []int{
	16, // "resource busy"
}

func MountDiskImageLinux(tb testing.TB, mountpoint, fsType string) {
	tb.Helper()

	imagePath := filepath.Join(tb.TempDir(), "test_image.img")

	const defaultPerms = 0o644

	imageFile := ts.NoErr(os.OpenFile(imagePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, defaultPerms))(tb)

	const mb = 1024 * 1024
	ts.NoErr(0, imageFile.Truncate(imageSizeMB*mb))(tb)
	ts.NoErr(0, imageFile.Close())(tb)

	mkfsCmd := exec.CommandContext(tb.Context(), "mkfs", "-t", fsType, imagePath)
	if out, err := mkfsCmd.CombinedOutput(); err != nil {
		tb.Fatalf("failed to create %v filesystem on disk image: %v, output: %s", fsType, err, string(out))
	}

	mountCmd := exec.CommandContext(tb.Context(), "sudo", "mount", "-o", "loop", "-t", fsType, imagePath, mountpoint)
	if out, err := mountCmd.CombinedOutput(); err != nil {
		tb.Fatalf("failed to mount disk image: %v, output: %s", err, string(out))
	}

	chownCmd := exec.CommandContext(tb.Context(), "sudo", "chown", strconv.Itoa(os.Getuid())+":"+strconv.Itoa(os.Getgid()), mountpoint)
	if out, err := chownCmd.CombinedOutput(); err != nil {
		tb.Fatalf("failed to chown mountpoint: %v, output: %s", err, string(out))
	}

	tb.Cleanup(func() {
		const (
			maxRetries   = 5
			initialDelay = 100 * time.Millisecond
		)

		for range ts.Backoff(maxRetries, initialDelay, time.Second) {
			unmountCmd := exec.CommandContext(tb.Context(), "sudo", "umount", mountpoint)
			if out, err := unmountCmd.CombinedOutput(); err != nil {
				tb.Logf("warning: failed to unmount disk image: %v, output: %s", err, string(out))
			} else {
				break
			}
		}
	})
}
