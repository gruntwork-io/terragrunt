package services

import (
	"errors"
	"fmt"
	"os"
)

// ErrCacheDirNotSpecified is returned by [ProviderService.Init] when the
// service was built without a cache directory, leaving it nowhere to write
// the providers it caches. Match with errors.Is.
var ErrCacheDirNotSpecified = errors.New("provider cache directory not specified")

// UnexpectedProviderCachePathError is returned when something other than a
// Terragrunt-managed symlink occupies a provider's package path inside the
// provider cache directory. Terragrunt only ever writes a directory (from a
// fresh download) or a symlink (pointing at the user plugins directory) to
// this path, so anything else is treated as user content and reported rather
// than removed.
type UnexpectedProviderCachePathError struct {
	Path string
	Mode os.FileMode
}

func (e *UnexpectedProviderCachePathError) Error() string {
	return fmt.Sprintf(
		"unexpected non-symlink at provider package path %q (mode %s); refusing to remove",
		e.Path,
		e.Mode,
	)
}

// ChecksumField names a field of a registry's provider download response that
// verifying the archive depends on.
type ChecksumField string

const (
	FieldSHA256Sum              ChecksumField = "shasum"
	FieldSHA256SumsURL          ChecksumField = "shasums_url"
	FieldSHA256SumsSignatureURL ChecksumField = "shasums_signature_url"
)

// ChecksumFieldMissingError is returned when a registry's download response
// for a provider leaves out a field that verifying the archive depends on.
type ChecksumFieldMissingError struct {
	Provider string
	Field    ChecksumField
}

func (e *ChecksumFieldMissingError) Error() string {
	return fmt.Sprintf(
		"registry response for provider %s does not include %q, so the provider archive cannot be verified",
		e.Provider,
		e.Field,
	)
}

// InvalidChecksumError is returned when the `shasum` in a registry's download
// response for a provider is not a hex-encoded SHA-256 hash.
type InvalidChecksumError struct {
	Provider string
}

func (e *InvalidChecksumError) Error() string {
	return fmt.Sprintf(
		"registry response for provider %s includes a %q that is not a hex-encoded SHA-256 hash",
		e.Provider,
		FieldSHA256Sum,
	)
}
