package commands_test

import (
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// testWorkDir is the working dir the wrapper tests run in, absolute on the host OS.
var testWorkDir = hostPath("/work")

// hostPath returns the slash-rooted path as an absolute path on the host OS, keeping any `.` or `..` elements.
func hostPath(path string) string {
	return strings.TrimSuffix(venvtest.Root("/"), string(filepath.Separator)) + filepath.FromSlash(path)
}
