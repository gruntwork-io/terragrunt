package cas

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
	// ErrGitClone is returned when the git clone operation fails
	ErrGitClone Error = "failed to complete git clone"
	// ErrNoTerraformBlock is returned when a terragrunt.hcl file has no terraform block
	ErrNoTerraformBlock Error = "no terraform block found"
	// ErrBlockNotFound is returned when a named block is not found in a stack file
	ErrBlockNotFound Error = "block not found"
	// ErrGetFileNotSupported is returned when GetFile is called on the CAS protocol getter
	ErrGetFileNotSupported Error = "CAS protocol does not support single file downloads"
	// ErrCASRefMissingPrefix is returned when a CAS reference is missing the expected hash algorithm prefix
	ErrCASRefMissingPrefix Error = "CAS reference missing expected hash algorithm prefix"
	// ErrCASRefEmptyHash is returned when a CAS reference has an empty hash
	ErrCASRefEmptyHash Error = "CAS reference has empty hash"
	// ErrTreeNotFound is returned when a tree hash is not found in any CAS store
	ErrTreeNotFound Error = "tree not found in CAS store"
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
)

func wrapError(op, path string, err error) error {
	return &WrappedError{
		Op:   op,
		Path: path,
		Err:  err,
	}
}
