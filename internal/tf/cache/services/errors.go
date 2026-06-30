package services

import (
	"fmt"
	"os"
)

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
	return fmt.Sprintf("unexpected non-symlink at provider package path %q (mode %s); refusing to remove", e.Path, e.Mode)
}
