package git

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// Error types that can be returned by the cas package
type Error string

func (e Error) Error() string {
	return string(e)
}

const (
	// ErrTempDir is returned when failing to create or close a temporary directory
	ErrTempDir Error = "failed to create or manage temporary directory"
	// ErrCreateDir is returned when failing to create a directory
	ErrCreateDir Error = "failed to create directory"
	// ErrReadFile is returned when failing to read a file
	ErrReadFile Error = "failed to read file"
	// ErrParseTree is returned when failing to parse git tree output
	ErrParseTree Error = "failed to parse git tree output"
	// ErrParseDiff is returned when failing to parse git diff output
	ErrParseDiff Error = "failed to parse git diff output"
	// ErrGitClone is returned when the git clone operation fails
	ErrGitClone Error = "failed to complete git clone"
	// ErrCreateTempDir is returned when failing to create a temporary directory
	ErrCreateTempDir Error = "failed to create temporary directory"
	// ErrCleanupTempDir is returned when failing to clean up a temporary directory
	ErrCleanupTempDir Error = "failed to clean up temporary directory"
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

// Git operation errors
var (
	ErrCommandSpawn        = errors.New("failed to spawn git command")
	ErrNoMatchingReference = errors.New("no matching reference")
	ErrReadTree            = errors.New("failed to read tree")
	ErrNoWorkDir           = errors.New("working directory not set")
	ErrNoGoRepo            = errors.New("go repository not set")
)
