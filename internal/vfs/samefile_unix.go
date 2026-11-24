//go:build !windows

package vfs

import (
	"io/fs"
	"os"
)

// sameFile reports whether a and b describe the same file, as two hard links to one inode do.
func sameFile(a, b fs.FileInfo) bool {
	return os.SameFile(a, b)
}
