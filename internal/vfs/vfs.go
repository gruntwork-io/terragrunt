// Package vfs provides a virtual filesystem abstraction for testing and production use.
// It wraps afero to provide a consistent interface for filesystem operations.
package vfs

import (
	"os"

	"github.com/spf13/afero"
)

// FS is the filesystem interface used throughout the codebase.
// It provides an abstraction over real and in-memory filesystems.
type FS = afero.Fs

// NewOSFS returns a filesystem backed by the real operating system filesystem.
func NewOSFS() FS {
	return afero.NewOsFs()
}

// NewMemMapFS returns an in-memory filesystem for testing purposes.
func NewMemMapFS() FS {
	return afero.NewMemMapFs()
}

// FileExists checks if a path exists using the given filesystem.
func FileExists(fs FS, path string) bool {
	_, err := fs.Stat(path)
	return err == nil
}

// WriteFile writes data to a file on the given filesystem.
func WriteFile(fs FS, filename string, data []byte, perm os.FileMode) error {
	return afero.WriteFile(fs, filename, data, perm)
}

// ReadFile reads the contents of a file from the given filesystem.
func ReadFile(fs FS, filename string) ([]byte, error) {
	return afero.ReadFile(fs, filename)
}

// Symlink creates a symbolic link. It uses afero's SymlinkIfPossible
// which is supported by both OsFs and MemMapFs.
func Symlink(fs FS, oldname, newname string) error {
	linker, ok := fs.(afero.Linker)
	if !ok {
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: afero.ErrNoSymlink}
	}

	return linker.SymlinkIfPossible(oldname, newname)
}
