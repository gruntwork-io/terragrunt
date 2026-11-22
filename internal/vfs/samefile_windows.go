//go:build windows

package vfs

import "io/fs"

// sameFile always reports false, so every pair of files is read.
//
// [os.SameFile] compares the 64-bit file IDs that Windows reports for a directory listing. ReFS file IDs are 128
// bits, and their 64-bit forms collide across directories, so [os.SameFile] reports distinct files as the same.
func sameFile(_, _ fs.FileInfo) bool {
	return false
}
