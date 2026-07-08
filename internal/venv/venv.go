// Package venv defines the root virtualized environment threaded from the
// Terragrunt binary entrypoint down through the CLI and its commands.
//
// A [Venv] bundles the side-effect handles every layer below the CLI needs
// to do its work: [vfs.FS] for filesystem reads and writes, [vexec.Exec]
// for spawning subprocesses, the shell environment variables read at
// startup, and the stdout/stderr writers. Production code constructs the
// real bundle once at the top via [OSVenv]; tests construct an in-memory
// bundle and drive the full CLI through it.
//
// This is the one Venv type threaded through the codebase. A package may
// define its own local Venv only when its handle set genuinely differs
// (for example internal/cas, which carries a filesystem and a Git runner).
package venv

import (
	"errors"
	"io"
	"maps"
	"os"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/writer"
)

// ErrVenvEnvUnset is the panic value [Venv.RequireEnv] raises when Env is
// nil. Production callers build the Venv through [OSVenv], so it points at a
// test that forgot to set Env rather than a runtime condition.
var ErrVenvEnvUnset = errors.New("venv.Venv.Env is required but unset")

// Venv is the root virtualized environment. It carries the filesystem,
// process-execution, environment-variable, and writer handles that every
// Terragrunt operation needs. Env is shared by reference across the run and
// mutated in place as provider-cache, hook, and inputs contributions resolve.
type Venv struct {
	FS      vfs.FS
	Exec    vexec.Exec
	Env     map[string]string
	Writers writer.Writers
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

// WithExec returns a copy of v whose process executor is exec.
func (v Venv) WithExec(exec vexec.Exec) Venv {
	v.Exec = exec

	return v
}

// WithHandler returns a copy of v whose executor is an in-memory exec driven
// by h, for the in-memory test bundles this package serves.
func (v Venv) WithHandler(h vexec.Handler) Venv {
	v.Exec = vexec.NewMemExec(h)

	return v
}

// WithFS returns a copy of v backed by fs.
func (v Venv) WithFS(fs vfs.FS) Venv {
	v.FS = fs

	return v
}

// WithEnv returns a copy of v whose shell environment is env. A nil env
// becomes an empty map so the result still satisfies [Venv.RequireEnv].
func (v Venv) WithEnv(env map[string]string) Venv {
	if env == nil {
		env = map[string]string{}
	}

	v.Env = env

	return v
}

// WithEnvCloned returns a copy of v whose Env is an independent clone. Fan-out
// paths that process units one at a time hand each unit a clone so
// per-unit mutations (obtained credentials, TF_VAR_* contributions) never
// leak into sibling units. A nil Env becomes an empty map, per [Venv.WithEnv].
func (v Venv) WithEnvCloned() Venv {
	return v.WithEnv(maps.Clone(v.Env))
}

// RequireEnv panics with [ErrVenvEnvUnset] when Env is nil, guarding
// functions that write into the shared environment.
func (v Venv) RequireEnv() {
	if v.Env == nil {
		panic(ErrVenvEnvUnset)
	}
}

// OSVenv builds the production [Venv]: the real OS filesystem, the real
// OS process executor, a snapshot of the OS environment, and stdout/stderr
// wired to the real OS streams.
func OSVenv() Venv {
	return Venv{
		FS:      vfs.NewOSFS(),
		Exec:    vexec.NewOSExec(),
		Env:     ParseEnviron(os.Environ()),
		Writers: writer.Writers{Writer: os.Stdout, ErrWriter: os.Stderr},
	}
}

// ParseEnviron turns os.Environ-style KEY=VALUE entries into a map, splitting
// on the first "=" after the leading byte. That leading byte is skipped so the
// Windows per-drive working-directory variables, whose names begin with "="
// (e.g. "=C:"), keep their names intact. Entries without a separator are dropped.
func ParseEnviron(environ []string) map[string]string {
	out := make(map[string]string, len(environ))

	for _, entry := range environ {
		if entry == "" {
			continue
		}

		i := strings.IndexByte(entry[1:], '=')
		if i < 0 {
			continue
		}

		out[entry[:i+1]] = entry[i+2:]
	}

	return out
}
