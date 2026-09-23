//go:build !windows

package vfs

import "os"

// readFileSharingDelete is a plain read. POSIX lets a file be renamed or
// removed while it is open, so every read already shares delete access.
func readFileSharingDelete(name string) ([]byte, error) {
	return os.ReadFile(name)
}
