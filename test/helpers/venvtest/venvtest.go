// Package venvtest builds in-memory [venv.Venv] values for tests. New seeds
// the mem defaults; callers refine individual handles with venv.Venv's fluent
// With methods (WithHandler, WithExec, WithFS, WithEnv). Production code builds
// venvs through [venv.OSVenv] instead.
package venvtest

import (
	"context"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/writer"
)

// New returns an in-memory venv: a no-op mem exec, an in-memory filesystem,
// an empty (non-nil) environment, and both writers wired to [io.Discard].
// Refine it with venv.Venv's fluent With methods.
func New() venv.Venv {
	return venv.Venv{
		Exec:    vexec.NewMemExec(func(context.Context, vexec.Invocation) vexec.Result { return vexec.Result{} }),
		FS:      vfs.NewMemMapFS(),
		Env:     map[string]string{},
		Writers: writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
	}
}
