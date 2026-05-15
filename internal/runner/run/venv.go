package run

import (
	"github.com/gruntwork-io/terragrunt/internal/tflint"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/writer"
)

// Venv is the virtualized environment threaded through the hook execution
// chain. It bundles the process-execution handle, the filesystem, the
// outbound HTTP handle (used by credential providers that build AWS SDK
// clients), the shell environment, and the stdout/stderr writers so
// callers supply them per call rather than the run package holding them
// as package-level state. The name avoids "Env" so it is not confused
// with shell environment variables.
type Venv struct {
	// Exec runs hook commands and the embedded tflint binary.
	Exec vexec.Exec
	// FS backs filesystem reads performed inside hooks, including
	// tflint's .tflint.hcl discovery.
	FS vfs.FS
	// HTTP is the outbound HTTP handle used by credential providers
	// (notably amazonsts) that build AWS SDK clients so their traffic
	// shares the same transport as the rest of Terragrunt.
	HTTP vhttp.Client
	// Env is the shell environment shared across hook, inputs, and
	// extra-args contributions during a run.
	Env map[string]string
	// Writers carries the stdout/stderr handles used while rendering hook
	// and run output. Mirrors [venv.Venv.Writers]: held as a pointer so
	// overrides flow through [writer.Writers.WithWriter] without sharing
	// mutation across shallow-copied Venvs.
	Writers *writer.Writers
}

// OSVenv builds the production [Venv]: real OS process execution, real
// OS filesystem, an OS environment snapshot, and the real OS streams.
func OSVenv() *Venv {
	return FromRoot(venv.OSVenv())
}

// FromRoot projects the root [venv.Venv] threaded from the CLI entrypoint
// into the run package's local Venv. The two carry the same handles but
// are distinct types so the run package owns its own contract.
func FromRoot(v *venv.Venv) *Venv {
	return &Venv{Exec: v.Exec, FS: v.FS, HTTP: v.HTTP, Env: v.Env, Writers: v.Writers}
}

// ToRoot is the inverse of [FromRoot]: it projects a run.Venv back into
// the root [venv.Venv] for callers (notably config.ParsingContext) that
// hold the root type.
func (v *Venv) ToRoot() *venv.Venv {
	return &venv.Venv{FS: v.FS, Exec: v.Exec, HTTP: v.HTTP, Env: v.Env, Writers: v.Writers}
}

// tflintVenv translates a run.Venv into the tflint package's Venv. The
// two carry the same handles but are distinct types so each package owns
// its own contract.
func (v *Venv) tflintVenv() *tflint.Venv {
	return &tflint.Venv{Exec: v.Exec, FS: v.FS, Env: v.Env, Writers: v.Writers}
}
