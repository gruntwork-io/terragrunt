package tflint

import (
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// Venv is the virtualized environment passed to tflint operations. It
// bundles the process-execution handle and the filesystem so callers
// supply both per call rather than the tflint package holding them as
// package-level state. The name avoids "Env" so it is not confused with
// shell environment variables.
type Venv struct {
	// Exec runs the tflint binary. Tests can substitute a stub via
	// [vexec.NewMemExec].
	Exec vexec.Exec
	// FS is the filesystem tflint reads through when searching for
	// .tflint.hcl and filtering optional var-files.
	FS vfs.FS
}

// OSVenv builds the production [Venv]: real OS process execution and the
// real OS filesystem.
func OSVenv() Venv {
	return Venv{Exec: vexec.NewOSExec(), FS: vfs.NewOSFS()}
}
