// Package stdout provides utilities for working with stdout.
package stdout

import (
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ShouldColor returns true if output written to stdout should be colored.
// Anything other than a terminal on the other end, a pipe, a file, or a
// character device such as /dev/null, reads escape sequences as text rather
// than as color, so it gets none.
func ShouldColor(l log.Logger, v *venv.Venv) bool {
	v.RequireTerminal()

	return !l.Formatter().DisabledColors() && v.Terminal.StdoutIsTTY()
}
