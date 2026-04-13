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
func MountDiskImageMacOS(t testing.TB, mountpoint, fsType string) {
	t.Helper()

	if runtime.GOOS != "darwin" {
		t.Fatalf("this only works on macOS")
	}

	// Create the disk image file path in the temp directory managed by TB
	imagePath := filepath.Join(t.TempDir(), "test_image.sparseimage")

	// Create the disk image file
	cmd := exec.Command("hdiutil", "create", "-size", strconv.Itoa(imageSizeMB)+"m", "-fs", fsType, "-volname", "TestVolume", "-type", "SPARSE", "-quiet", imagePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create disk image: %v, output: %s", err, string(out))
	}

	// Mount the disk image to the provided location
	mountCmd := exec.Command("hdiutil", "attach", "-mountPoint", mountpoint, imagePath)
	if err := mountCmd.Run(); err != nil {
		t.Fatalf("failed to mount disk image: %v", err)
	}

	// Register cleanup to unmount the disk after the test
	// this may not be necessary since it seemse deleting the image and mount location does this automatically
	t.Cleanup(func() {
		for attempt := range ts.Backoff(4, 100*time.Millisecond, time.Second) {
			unmountCmd := exec.Command("hdiutil", "detach", mountpoint)
			if out, err := unmountCmd.CombinedOutput(); err != nil {
				if slices.Contains(retryableDetachExitCodes, unmountCmd.ProcessState.ExitCode()) {
					t.Logf("warning: failed to unmount disk image (attempt %d): %v, output: %s", attempt+1, err, string(out))
					t.Logf("if this was \"No such file or directory\", it may be safe to ignore as the test cleanup may have just deleted the file and mount point")
					continue
				}
				t.Logf("failed to unmount disk image: %v, output: %s", err, string(out))
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

func MountDiskImageLinux(t testing.TB, mountpoint, fsType string) {
	t.Helper()

	imagePath := filepath.Join(t.TempDir(), "test_image.img")

	imageFile := ts.NoErr(os.OpenFile(imagePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644))(t)

	ts.NoErr(0, imageFile.Truncate(imageSizeMB*1024*1024))(t)
	ts.NoErr(0, imageFile.Close())(t)

	mkfsCmd := exec.Command("mkfs", "-t", fsType, imagePath)
	if out, err := mkfsCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create %v filesystem on disk image: %v, output: %s", fsType, err, string(out))
	}

	mountCmd := exec.Command("sudo", "mount", "-o", "loop", "-t", fsType, imagePath, mountpoint)
	if out, err := mountCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to mount disk image: %v, output: %s", err, string(out))
	}

	chownCmd := exec.Command("sudo", "chown", strconv.Itoa(os.Getuid())+":"+strconv.Itoa(os.Getgid()), mountpoint)
	if out, err := chownCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to chown mountpoint: %v, output: %s", err, string(out))
	}

	t.Cleanup(func() {
		for range ts.Backoff(5, 100*time.Millisecond, time.Second) {
			unmountCmd := exec.Command("sudo", "umount", mountpoint)
			if out, err := unmountCmd.CombinedOutput(); err != nil {
				t.Logf("warning: failed to unmount disk image: %v, output: %s", err, string(out))
			} else {
				break
			}
		}
	})
}
