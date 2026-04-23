package generate

import (
	stderrors "errors"
)

// WorkingDirNotDirectoryError is returned when --working-dir exists but points
// to a regular file or other non-directory entry.
type WorkingDirNotDirectoryError struct {
	path string
}

func NewWorkingDirNotDirectoryError(path string) *WorkingDirNotDirectoryError {
	return &WorkingDirNotDirectoryError{
		path: path,
	}
}

func (err WorkingDirNotDirectoryError) Error() string {
	return "working directory is not a directory: " + err.path
}

// Is reports a match for ANY WorkingDirNotDirectoryError value. Paths are
// not compared: this mirrors the typical sentinel pattern so callers can
// write errors.Is(err, ErrWorkingDirNotDirectory). Callers that need to
// inspect the path should cast to *WorkingDirNotDirectoryError directly.
func (err WorkingDirNotDirectoryError) Is(target error) bool {
	_, ok := target.(*WorkingDirNotDirectoryError)
	return ok
}

// ErrWorkingDirNotDirectory is a sentinel for WorkingDirNotDirectoryError
// for backward compatibility with existing tests.
var ErrWorkingDirNotDirectory = &WorkingDirNotDirectoryError{}

func IsWorkingDirNotDirectoryError(err error) bool {
	return stderrors.Is(err, ErrWorkingDirNotDirectory)
}
