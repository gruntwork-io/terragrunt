package run

import (
	"github.com/gruntwork-io/terragrunt/internal/tflint"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// Venv is the virtualized environment threaded through the hook execution
// chain. It bundles the process-execution handle and the filesystem so
// callers supply both per call rather than the run package holding them as
// package-level state. The name avoids "Env" so it is not confused with
// shell environment variables.
type Venv struct {
	// Exec runs hook commands and the embedded tflint binary.
	Exec vexec.Exec
	// FS backs filesystem reads performed inside hooks, including
	// tflint's .tflint.hcl discovery.
	FS vfs.FS
}

// OSVenv builds the production [Venv]: real OS process execution and the
// real OS filesystem.
func OSVenv() Venv {
	return Venv{Exec: vexec.NewOSExec(), FS: vfs.NewOSFS()}
}

// tflintVenv translates a run.Venv into the tflint package's Venv. The
// two carry the same handles but are distinct types so each package owns
// its own contract.
func (v Venv) tflintVenv() tflint.Venv {
	return tflint.Venv{Exec: v.Exec, FS: v.FS}
}
