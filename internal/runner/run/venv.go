package run

import (
	"io"

	"github.com/gruntwork-io/terragrunt/internal/tflint"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/writer"
)

// Venv is the virtualized environment threaded through the hook execution
// chain. It bundles the process-execution handle, the filesystem, the
// shell environment, and the stdout/stderr writers so callers supply them
// per call rather than the run package holding them as package-level
// state. The name avoids "Env" so it is not confused with shell
// environment variables. Env is shared by reference across the run and
// mutated in place as hook, inputs, and extra-args contributions resolve.
type Venv struct {
	Exec    vexec.Exec
	FS      vfs.FS
	Env     map[string]string
	Writers writer.Writers
}

// OSVenv builds the production [Venv]: real OS process execution, real
// OS filesystem, an OS environment snapshot, and the real OS streams.
func OSVenv() Venv {
	return FromRoot(venv.OSVenv())
}

// FromRoot projects the root [venv.Venv] threaded from the CLI entrypoint
// into the run package's local Venv.
func FromRoot(v venv.Venv) Venv {
	return Venv{Exec: v.Exec, FS: v.FS, Env: v.Env, Writers: v.Writers}
}

// ToRoot is the inverse of [FromRoot], for callers (notably
// config.ParsingContext) that hold the root type.
func (v Venv) ToRoot() venv.Venv {
	return venv.Venv{FS: v.FS, Exec: v.Exec, Env: v.Env, Writers: v.Writers}
}

// tflintVenv translates a run.Venv into the tflint package's Venv.
func (v Venv) tflintVenv() tflint.Venv {
	return tflint.Venv{Exec: v.Exec, FS: v.FS, Env: v.Env, Writers: v.Writers}
}

// RequireEnv panics with [venv.ErrVenvEnvUnset] when Env is nil, guarding
// functions that write into the shared environment.
func (v Venv) RequireEnv() {
	if v.Env == nil {
		panic(venv.ErrVenvEnvUnset)
	}
}

// WithEnvCloned returns a copy of v whose Env is an independent clone, so
// per-unit mutations never leak into sibling units. See [venv.Venv.WithEnvCloned].
func (v Venv) WithEnvCloned() Venv {
	return FromRoot(v.ToRoot().WithEnvCloned())
}

// WithWriter returns a copy of v whose primary writer is w.
func (v Venv) WithWriter(w io.Writer) Venv {
	v.Writers.Writer = w

	return v
}

// WithErrWriter returns a copy of v whose error writer is w.
func (v Venv) WithErrWriter(w io.Writer) Venv {
	v.Writers.ErrWriter = w

	return v
}
