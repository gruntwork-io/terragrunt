package glob

import "github.com/bmatcuk/doublestar/v4"

// ErrBadPattern is returned for a malformed pattern in the syntax of the fileset function.
var ErrBadPattern = doublestar.ErrBadPattern

// ValidOpenTofuPattern reports whether pattern is well formed in the syntax of the fileset function.
func ValidOpenTofuPattern(pattern string) bool {
	return doublestar.ValidatePattern(pattern)
}

// OpenTofuMatch reports whether name matches pattern in the syntax of the fileset function.
func OpenTofuMatch(pattern, name string) (bool, error) {
	return doublestar.Match(pattern, name)
}
