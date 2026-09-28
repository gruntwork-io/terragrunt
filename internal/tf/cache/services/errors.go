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
