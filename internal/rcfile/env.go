package rcfile

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// urlSeparator marks a value as a URL rather than a path.
	urlSeparator = "://"

	// currentDirPrefix and parentDirPrefix mark a value as a path that is relative to the
	// directory holding the rc file.
	currentDirPrefix = "./"
	parentDirPrefix  = "../"
)

// envValue renders a declared environment variable value.
//
// The value is expanded with os.ExpandEnv first, so it can reference variables that are
// already in the environment. A value that starts with "./" or "../" is then resolved
// against baseDir, the directory holding the rc file, so that a configuration committed to
// a repository keeps working no matter where the repository is cloned. A value that
// contains "://" is left alone: it is a URL, not a path.
func envValue(baseDir, val string) string {
	val = os.ExpandEnv(val)

	if strings.Contains(val, urlSeparator) {
		return val
	}

	if !strings.HasPrefix(val, currentDirPrefix) && !strings.HasPrefix(val, parentDirPrefix) {
		return val
	}

	return filepath.Join(baseDir, filepath.Clean(val))
}
