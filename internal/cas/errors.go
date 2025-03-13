package cas

import (
	"fmt"

	// I'm intentionally not using github.com/gruntwork-io/terragrunt/internal/errors
	// here. I want to construct raw errors so that they can be wrapped later with their
	// relevant stack traces when returned from locations where they're used.
	"errors"
)

// Error types that can be returned by the cas package
var (
	// ErrTempDir is returned when failing to create or close a temporary directory
	ErrTempDir = errors.New("failed to create or manage temporary directory")
	// ErrCreateDir is returned when failing to create a directory
	ErrCreateDir = errors.New("failed to create directory")
	// ErrReadFile is returned when failing to read a file
	ErrReadFile = errors.New("failed to read file")
	// ErrParseTree is returned when failing to parse git tree output
	ErrParseTree = errors.New("failed to parse git tree output")
	// ErrGitClone is returned when the git clone operation fails
	ErrGitClone = errors.New("failed to complete git clone")
	// ErrCreateTempDir is returned when failing to create a temporary directory
	ErrCreateTempDir = errors.New("failed to create temporary directory")
	// ErrCleanupTempDir is returned when failing to clean up a temporary directory
	ErrCleanupTempDir = errors.New("failed to clean up temporary directory")
	// ErrCommandSpawn is returned when failing to spawn a git command
	ErrCommandSpawn = errors.New("failed to spawn git command")
	// ErrNoMatchingReference is returned when no matching reference is found
	ErrNoMatchingReference = errors.New("no matching reference")
	// ErrReadTree is returned when failing to read a git tree
	ErrReadTree = errors.New("failed to read tree")
	// ErrNoWorkDir is returned when a working directory is not set
	ErrNoWorkDir = errors.New("working directory not set")
)

// WrappedError provides additional context for errors
type WrappedError struct {
	Op      string // Operation that failed
	Path    string // File path if applicable
	Err     error  // Original error
	Context string // Additional context
}

func (e *WrappedError) Error() string {
	if e.Context != "" {
		return fmt.Sprintf("%s: %s: %v", e.Op, e.Context, e.Err)
	}

	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *WrappedError) Unwrap() error {
	return e.Err
}

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
