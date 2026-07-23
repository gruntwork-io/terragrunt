// Package venvtest builds in-memory [venv.Venv] values for tests. New seeds
// the mem defaults; callers refine individual handles with venv.Venv's fluent
// With methods (WithHandler, WithExec, WithFS, WithSops, WithEnv, WithGOOS,
// WithUserHomeDir). Production code builds venvs through [venv.OSVenv] instead.
package venvtest

import (
	"context"
	"io"
	"runtime"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/internal/writer"
)

// New returns an in-memory venv: a no-op mem exec, an in-memory filesystem,
// a mem SOPS decrypter yielding empty cleartext, an empty (non-nil)
// environment, deterministic platform handles, and both writers wired to
// [io.Discard]. Refine it with venv.Venv's fluent With methods.
func New() venv.Venv {
	return venv.Venv{
		Exec: vexec.NewMemExec(
			func(context.Context, vexec.Invocation) vexec.Result { return vexec.Result{} },
		),
		FS:   vfs.NewMemMapFS(),
		Sops: vsops.NewMemDecrypter(func(string, string) ([]byte, error) { return nil, nil }),
		Env:  map[string]string{},
		Platform: &venv.Platform{
			UserHomeDir: func() (string, error) {
				return "", nil
			},
			GOOS: runtime.GOOS,
		},
		Writers: &writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
	}
}
