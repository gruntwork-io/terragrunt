package tflint

import (
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/writer"
)

// Venv is the virtualized environment passed to tflint operations. It
// bundles the process-execution handle, the filesystem, the shell
// environment, and the stdout/stderr writers so callers supply them per
// call rather than the tflint package holding them as package-level
// state. The name avoids "Env" so it is not confused with shell
// environment variables.
type Venv struct {
	Exec    vexec.Exec
	FS      vfs.FS
	Env     map[string]string
	Writers writer.Writers
}

// OSVenv builds the production [Venv] from a real-OS [venv.OSVenv].
func OSVenv() Venv {
	return FromRoot(venv.OSVenv())
}

// FromRoot projects the root [venv.Venv] threaded from the CLI entrypoint
// into the tflint package's local Venv.
func FromRoot(v venv.Venv) Venv {
	return Venv{Exec: v.Exec, FS: v.FS, Env: v.Env, Writers: v.Writers}
}

// ToRoot is the inverse of [FromRoot], for callers that hold the root type.
func (v Venv) ToRoot() venv.Venv {
	return venv.Venv{FS: v.FS, Exec: v.Exec, Env: v.Env, Writers: v.Writers}
}
