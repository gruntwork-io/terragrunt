package cln

import (
	"errors"
	"fmt"
)

// Error types that can be returned by the cln package
type Error string

func (e Error) Error() string {
	return string(e)
}

const (
	// ErrTempDir is returned when failing to create or close a temporary directory
	ErrTempDir Error = "failed to create or manage temporary directory"
	// ErrCreateDir is returned when failing to create a directory
	ErrCreateDir Error = "failed to create directory"
	// ErrHomeDir is returned when failing to find home directory
	ErrHomeDir Error = "failed to find home directory"
	// ErrWriteToStore is returned when failing to write to the cln store
	ErrWriteToStore Error = "failed to write to cln-store"
	// ErrHardLink is returned when failing to create a hard link
	ErrHardLink Error = "failed to create hard link"
	// ErrParseMode is returned when failing to parse file mode
	ErrParseMode Error = "failed to parse file mode"
	// ErrReadFile is returned when failing to read a file
	ErrReadFile Error = "failed to read file"
	// ErrParseTree is returned when failing to parse git tree output
	ErrParseTree Error = "failed to parse git tree output"
	// ErrGitClone is returned when the git clone operation fails
	ErrGitClone Error = "failed to complete git clone"
)

// WrappedError provides additional context for errors
type WrappedError struct {
	Op      string // Operation that failed
	Path    string // File path if applicable
	Err     error  // Original error
	Context string // Additional context
}

func (e *WrappedError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s: %s: %v", e.Op, e.Path, e.Err)
	}
	if e.Context != "" {
		return fmt.Sprintf("%s: %s: %v", e.Op, e.Context, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *WrappedError) Unwrap() error {
	return e.Err
}

// Git operation errors
var (
	ErrCommandSpawn        = errors.New("failed to spawn git command")
	ErrNoMatchingReference = errors.New("no matching reference")
	ErrReadTree            = errors.New("failed to read tree")
	ErrNoWorkDir           = errors.New("working directory not set")
)

func wrapError(op, path string, err error) error {
	return &WrappedError{
		Op:   op,
		Path: path,
		Err:  err,
	}
}

func wrapErrorWithContext(op, context string, err error) error {
	return &WrappedError{
		Op:      op,
		Context: context,
		Err:     err,
	}
}
