package clngo

import "fmt"

// Error types that can be returned by the clngo package
type Error string

func (e Error) Error() string {
	return string(e)
}

const (
	// ErrTempDir is returned when failing to create or close a temporary directory
	ErrTempDir Error = "failed to create or manage temporary directory"
	// ErrCommandSpawn is returned when failing to spawn a git command
	ErrCommandSpawn Error = "failed to spawn git command"
	// ErrGitClone is returned when the git clone operation fails
	ErrGitClone Error = "failed to complete git clone"
	// ErrCreateDir is returned when failing to create a directory
	ErrCreateDir Error = "failed to create directory"
	// ErrHomeDir is returned when failing to find home directory
	ErrHomeDir Error = "failed to find home directory"
	// ErrNoMatchingReference is returned when no matching git reference is found
	ErrNoMatchingReference Error = "no matching reference found"
	// ErrWriteToStore is returned when failing to write to the cln store
	ErrWriteToStore Error = "failed to write to cln-store"
	// ErrHardLink is returned when failing to create a hard link
	ErrHardLink Error = "failed to create hard link"
	// ErrReadTree is returned when failing to read a git tree
	ErrReadTree Error = "failed to read tree"
	// ErrParseMode is returned when failing to parse file mode
	ErrParseMode Error = "failed to parse file mode"
	// ErrReadFile is returned when failing to read a file
	ErrReadFile Error = "failed to read file"
	// ErrParseTree is returned when failing to parse git tree output
	ErrParseTree Error = "failed to parse git tree output"
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
