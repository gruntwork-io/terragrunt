package vfs

import (
	"errors"
	"fmt"
)

// ErrNoHardLink is returned when a filesystem does not support hard links.
var ErrNoHardLink = errors.New("hard link not supported")

// ErrNoLock is returned when a filesystem does not support locking.
var ErrNoLock = errors.New("locking not supported")

// ErrSymlinkEscapes is returned by [ValidateSymlinkTarget] and
// [ValidateResolvedSymlinkTarget] when a link leads outside the directory it
// was checked against. Match with errors.Is to tell an escape from the
// resolution failure [ValidateResolvedSymlinkTarget] returns for a chain that
// cannot be followed at all.
var ErrSymlinkEscapes = errors.New("symlink target escapes destination")

// PathIsNotDirectory is returned when the given path is unexpectedly not a directory.
type PathIsNotDirectory struct {
	path string
}

func (err PathIsNotDirectory) Error() string {
	return err.path + " is not a directory"
}

// ZipDecompressedSizeLimitError reports an extraction exceeding its configured decompressed size limit.
type ZipDecompressedSizeLimitError struct {
	// Name is the archive entry whose extraction breached the limit.
	Name string
	// Size is the entry's declared uncompressed size in bytes.
	Size uint64
	// Limit is the configured total decompressed size limit in bytes.
	Limit int64
}

func (err ZipDecompressedSizeLimitError) Error() string {
	return fmt.Sprintf(
		"extracting file %q breached the total decompressed size limit of %d (entry size %d)",
		err.Name, err.Limit, err.Size,
	)
}
