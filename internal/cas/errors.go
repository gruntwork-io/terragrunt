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
	// ErrAbsoluteSource is returned when an update_source_with_cas source is an absolute path
	ErrAbsoluteSource Error = "update_source_with_cas does not support absolute sources"
	// ErrSourceEscapesRepo is returned when an update_source_with_cas source resolves outside the cloned repository
	ErrSourceEscapesRepo Error = "update_source_with_cas source escapes repository root"
	// ErrNotADirectory is returned when a path expected to be a directory is not.
	ErrNotADirectory Error = "not a directory"
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
	ErrCommandSpawn         = errors.New("failed to spawn git command")
	ErrNoMatchingReference  = errors.New("no matching reference")
	ErrReadTree             = errors.New("failed to read tree")
	ErrNoWorkDir            = errors.New("working directory not set")
	ErrGitStorePath         = errors.New("failed to prepare git store path")
	ErrGitStoreLock         = errors.New("failed to acquire git store lock")
	ErrGitStoreFSNotOS      = errors.New("git store requires an OS-backed filesystem")
	ErrFallbackCloneDir     = errors.New("failed to create fallback clone directory")
	ErrFetchClosureRequired = errors.New("fetch closure is required")
)

// UpdateSourceWithCASRequiresCASError is returned when a block sets
// update_source_with_cas = true but CAS is unavailable, either because the
// experiment is not enabled or because --no-cas is set. The relative source
// has no meaning without CAS, so Terragrunt rejects the configuration rather
// than silently falling through to the standard getter.
type UpdateSourceWithCASRequiresCASError struct {
	// BlockType is the kind of block carrying the attribute: "unit", "stack", or "terraform".
	BlockType string
	// Name is the block label. Empty for "terraform" blocks, which are unlabeled.
	Name string
	// Path is the file containing the offending block.
	Path string
}

// GitStoreObjectMissingError is returned when a fetch into the central git
// store completes but the requested object is still not reachable. Callers
// fall back to a temporary clone when they see this.
type GitStoreObjectMissingError struct {
	Hash string
	Ref  string
	URL  string
}

func (e *GitStoreObjectMissingError) Error() string {
	return fmt.Sprintf(
		"object %s not present in central git store after fetching %s from %s",
		e.Hash, e.Ref, e.URL,
	)
}

func (e *UpdateSourceWithCASRequiresCASError) Error() string {
	subject := e.BlockType
	if e.Name != "" {
		subject = fmt.Sprintf("%s %q", e.BlockType, e.Name)
	}

	return fmt.Sprintf(
		"%s in %s sets update_source_with_cas = true, which requires "+
			"the 'cas' experiment to be enabled and the --no-cas flag to be unset",
		subject, e.Path,
	)
}
