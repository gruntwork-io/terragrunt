// Package venv defines the root virtualized environment threaded from the
// Terragrunt binary entrypoint down through the CLI and its commands.
//
// A [Venv] bundles the side-effect handles every layer below the CLI needs
// to do its work: [vfs.FS] for filesystem reads and writes, [vexec.Exec]
// for spawning subprocesses, [vhttp.Client] for outbound HTTP,
// [vsops.Decrypter] for SOPS decryption, the shell environment variables and
// platform handles read at startup, and the stdout/stderr writers. Production
// code constructs the real bundle once at the top via [OSVenv]; tests
// construct an in-memory bundle and drive the full CLI through it.
//
// This is the one Venv type threaded through the codebase. A package may
// define its own local Venv only when its handle set genuinely differs
// from what this bundle carries.
package venv

import (
	"errors"
	"io"
	"maps"
	"os"
	"runtime"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/internal/writer"
)

// ErrVenvEnvUnset is the panic value [Venv.RequireEnv] raises when Env is
// nil. Production callers build the Venv through [OSVenv], so it points at a
// test that forgot to set Env rather than a runtime condition.
var ErrVenvEnvUnset = errors.New("venv.Venv.Env is required but unset")

// ErrVenvFSUnset is the panic value [Venv.RequireFS] raises when FS is nil.
// Production callers build the Venv through [OSVenv], so it points at a test
// that forgot to set FS rather than a runtime condition.
var ErrVenvFSUnset = errors.New("venv.Venv.FS is required but unset")

// ErrVenvExecUnset is the panic value [Venv.RequireExec] raises when Exec is
// nil. Production callers build the Venv through [OSVenv], so it points at a
// test that forgot to set Exec rather than a runtime condition.
var ErrVenvExecUnset = errors.New("venv.Venv.Exec is required but unset")

// ErrVenvHTTPUnset is the panic value [Venv.RequireHTTP] raises when HTTP is
// nil. Production callers build the Venv through [OSVenv], so it points at a
// test that forgot to set HTTP rather than a runtime condition.
var ErrVenvHTTPUnset = errors.New("venv.Venv.HTTP is required but unset")

// ErrVenvGOOSUnset is the panic value [Venv.RequireGOOS] raises when GOOS is empty.
var ErrVenvGOOSUnset = errors.New("venv.Venv.Platform.GOOS is required but unset")

// ErrVenvUserHomeDirUnset is the panic value [Venv.RequireUserHomeDir] raises
// when UserHomeDir is nil.
var ErrVenvUserHomeDirUnset = errors.New("venv.Venv.Platform.UserHomeDir is required but unset")

// Platform carries the operating-system handles used below the CLI boundary.
type Platform struct {
	UserHomeDir func() (string, error)
	GOOS        string
}

// Venv is the root virtualized environment. It carries the filesystem,
// process-execution, HTTP, SOPS-decryption, environment-variable, platform,
// and writer handles that every Terragrunt operation needs. Env is shared by
// reference across the run and mutated in place as provider-cache, hook, and
// inputs contributions resolve. Writers is held as a pointer so per-call
// overrides via [writer.Writers.WithWriter] and [writer.Writers.WithErrWriter]
// produce fresh pointers without mutating the caller's value; never mutate its
// fields in place, since shallow-copied Venvs share the pointer.
type Venv struct {
	FS       vfs.FS
	Exec     vexec.Exec
	HTTP     vhttp.Client
	Sops     vsops.Decrypter
	Env      map[string]string
	Platform *Platform
	Writers  *writer.Writers
}

// WithWriter returns a copy of v whose primary writer is w. The copy gets
// a fresh Writers pointer so the caller's venv is untouched.
func (v Venv) WithWriter(w io.Writer) Venv {
	v.Writers = v.Writers.WithWriter(w)

	return v
}

// WithErrWriter returns a copy of v whose error writer is w. The copy gets
// a fresh Writers pointer so the caller's venv is untouched.
func (v Venv) WithErrWriter(w io.Writer) Venv {
	v.Writers = v.Writers.WithErrWriter(w)

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

// WithSops returns a copy of v whose SOPS decrypter is d.
func (v Venv) WithSops(d vsops.Decrypter) Venv {
	v.Sops = d

	return v
}

// WithFS returns a copy of v backed by fs.
func (v Venv) WithFS(fs vfs.FS) Venv {
	v.FS = fs

	return v
}

// WithGOOS returns a copy of v whose operating-system identifier is goos.
func (v Venv) WithGOOS(goos string) Venv {
	platform := Platform{}
	if v.Platform != nil {
		platform = *v.Platform
	}

	platform.GOOS = goos
	v.Platform = &platform

	return v
}

// WithUserHomeDir returns a copy of v whose home-directory lookup is userHomeDir.
func (v Venv) WithUserHomeDir(userHomeDir func() (string, error)) Venv {
	platform := Platform{}
	if v.Platform != nil {
		platform = *v.Platform
	}

	platform.UserHomeDir = userHomeDir
	v.Platform = &platform

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

// RequireFS panics with [ErrVenvFSUnset] when FS is nil. Functions that
// touch the filesystem call this as their first statement so a missing
// handle panics at the offending call site instead of inside an unrelated
// stack frame.
func (v Venv) RequireFS() {
	if v.FS == nil {
		panic(ErrVenvFSUnset)
	}
}

// RequireExec panics with [ErrVenvExecUnset] when Exec is nil. Functions
// that spawn subprocesses call this as their first statement so a missing
// handle panics at the offending call site instead of inside an unrelated
// stack frame.
func (v Venv) RequireExec() {
	if v.Exec == nil {
		panic(ErrVenvExecUnset)
	}
}

// RequireHTTP panics with [ErrVenvHTTPUnset] when HTTP is nil. Functions
// that probe over HTTP call this as their first statement so a missing
// handle panics at the offending call site instead of inside an unrelated
// stack frame.
func (v Venv) RequireHTTP() {
	if v.HTTP == nil {
		panic(ErrVenvHTTPUnset)
	}
}

// RequireGOOS panics with [ErrVenvGOOSUnset] when GOOS is empty.
func (v Venv) RequireGOOS() {
	if v.Platform == nil || v.Platform.GOOS == "" {
		panic(ErrVenvGOOSUnset)
	}
}

// RequireUserHomeDir panics with [ErrVenvUserHomeDirUnset] when UserHomeDir is nil.
func (v Venv) RequireUserHomeDir() {
	if v.Platform == nil || v.Platform.UserHomeDir == nil {
		panic(ErrVenvUserHomeDirUnset)
	}
}

// OSVenv builds the production [Venv]: the real OS filesystem, the real OS
// process executor, the real outbound HTTP client, platform handles, a
// snapshot of the OS environment, and stdout/stderr wired to the real OS
// streams.
//
// It returns a *[Venv] so the bundle is threaded by pointer through every
// downstream call — small parameter, no copying. Shallow-copying a
// pointed-to [Venv] (via `local := *v`) still shares the Env map with the
// original, so callers must go through [Venv.WithEnvCloned] before mutating
// environment variables; writer swaps stay independent because
// [writer.Writers.WithWriter] returns a fresh copy.
func OSVenv() *Venv {
	return &Venv{
		FS:   vfs.NewOSFS(),
		Exec: vexec.NewOSExec(),
		HTTP: vhttp.NewOSClient(),
		Sops: vsops.NewOSDecrypter(),
		Env:  ParseEnviron(os.Environ()),
		Platform: &Platform{
			UserHomeDir: os.UserHomeDir,
			GOOS:        runtime.GOOS,
		},
		Writers: &writer.Writers{Writer: os.Stdout, ErrWriter: os.Stderr},
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
