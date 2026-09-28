package git

import (
	"fmt"

	"errors"
)

// Error types that can be returned by the cas package
type Error string

func (e Error) Error() string {
	return string(e)
}

const (
	// ErrParseTree is returned when failing to parse git tree output
	ErrParseTree Error = "failed to parse git tree output"
	// ErrParseDiff is returned when failing to parse git diff output
	ErrParseDiff Error = "failed to parse git diff output"
	// ErrGitClone is returned when the git clone operation fails
	ErrGitClone Error = "failed to complete git clone"
	// ErrGitInitBare is returned when initializing a bare repository fails
	ErrGitInitBare Error = "failed to initialize bare git repository"
	// ErrGitFetch is returned when a git fetch operation fails
	ErrGitFetch Error = "failed to complete git fetch"
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
	ErrUnknownRevision     = errors.New("unknown revision")

	// ErrCatFileMissing reports that `git cat-file --batch` knows no object by
	// the requested name.
	ErrCatFileMissing = errors.New("object not found")
	// ErrCatFileAmbiguous reports that the requested name abbreviates more
	// than one object known to `git cat-file --batch`.
	ErrCatFileAmbiguous = errors.New("object name is ambiguous")
	// ErrCatFileNotBlob reports that the requested object exists but is not a
	// blob.
	ErrCatFileNotBlob = errors.New("object is not a blob")
	// ErrCatFileFraming reports a `git cat-file --batch` response that does not
	// follow the framing git-cat-file(1) documents.
	ErrCatFileFraming = errors.New("malformed git cat-file --batch response")
	// ErrCatFileShortRead reports a `git cat-file --batch` stream that ended
	// before the requested object was delivered in full.
	ErrCatFileShortRead = errors.New("git cat-file --batch ended before delivering the object")
)
