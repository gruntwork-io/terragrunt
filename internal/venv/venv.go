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
// Downstream packages (for example internal/runner/run and internal/tflint)
// keep their own package-local Venv types so each owns its own contract.
// They convert from this root via a FromRoot constructor.
package venv

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/writer"
)

// ErrVenvEnvUnset is the panic value [Venv.RequireEnv] raises when a
// function declares it mutates v.Env and the caller hands in a Venv with
// Env == nil. Production callers build Venv through [OSVenv], so the panic
// surfaces a test misconfiguration rather than a runtime condition.
var ErrVenvEnvUnset = errors.New("venv.Venv.Env is required but unset")

// Venv is the root virtualized environment. It carries the filesystem,
// process-execution, environment-variable, and writer handles that every
// Terragrunt operation needs.
type Venv struct {
	// FS backs every filesystem read and write.
	FS vfs.FS
	// Exec spawns every subprocess: tofu, terraform, git, hooks,
	// external auth providers, tflint.
	Exec vexec.Exec
	// Env holds the shell environment variables read at startup and is
	// mutated as Terragrunt resolves provider-cache, hook, and inputs
	// env contributions. The map is shared by reference across the run.
	Env map[string]string
	// Writers groups the stdout and stderr handles plus the log-formatting
	// flags that travel together through ParsingContext, shell options,
	// backend options, and the engine.
	Writers writer.Writers
}

// WithWriter returns a copy of v whose primary writer is w. The Env map and
// other handles are shared by reference with the receiver.
func (v Venv) WithWriter(w io.Writer) Venv {
	v.Writers.Writer = w

	return v
}

// WithErrWriter returns a copy of v whose error writer is w. The Env map and
// other handles are shared by reference with the receiver.
func (v Venv) WithErrWriter(w io.Writer) Venv {
	v.Writers.ErrWriter = w

	return v
}

// WithExec returns a copy of v whose process executor is exec. The Env map
// and other handles are shared by reference with the receiver.
func (v Venv) WithExec(exec vexec.Exec) Venv {
	v.Exec = exec

	return v
}

// WithHandler returns a copy of v whose executor is an in-memory exec driven
// by h. It serves the in-memory test bundles this package is built for;
// production code wires a real executor through OSVenv.
func (v Venv) WithHandler(h vexec.Handler) Venv {
	v.Exec = vexec.NewMemExec(h)

	return v
}

// WithFS returns a copy of v backed by fs. The Env map and other handles are
// shared by reference with the receiver.
func (v Venv) WithFS(fs vfs.FS) Venv {
	v.FS = fs

	return v
}

// WithEnv returns a copy of v whose shell environment is env. A nil env is
// replaced with an empty map so the result still satisfies RequireEnv.
func (v Venv) WithEnv(env map[string]string) Venv {
	if env == nil {
		env = map[string]string{}
	}

	v.Env = env

	return v
}

// RequireEnv panics with [ErrVenvEnvUnset] when v.Env is nil. Functions
// that write into the shared environment call this as their first
// statement so the contract sits next to the signature.
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
