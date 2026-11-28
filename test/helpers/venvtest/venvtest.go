// Package venvtest builds [venv.Venv] values for tests that need part of the
// real system. Start from [venv.MemVenv] when nothing has to be real; refine
// individual handles with venv.Venv's fluent With methods (WithHandler,
// WithExec, WithFS, WithSops, WithBrowser, WithEnv, WithGOOS, WithGOARCH,
// WithUserHomeDir, WithUserConfigDir). Production code builds venvs through
// [venv.OSVenv] instead.
//
// [NewFS] and [LoadFS] build the filesystems those venvs carry, from literals
// and from an on-disk fixture tree.
package venvtest

import (
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// NewWithOSFS returns [venv.MemVenv] with the real filesystem swapped in, for
// tests whose fixtures live on disk but which need nothing else live.
func NewWithOSFS() *venv.Venv {
	return venv.MemVenv().WithFS(vfs.NewOSFS())
}

// NewOSOverlayFS returns the real filesystem under an in-memory write layer,
// for tests whose subject reads fixtures on disk and must not change them. A
// write the subject makes lands in memory, and removing or renaming what is
// on disk fails; [vfs.NewOverlayFS] has the details.
func NewOSOverlayFS() vfs.FS {
	return vfs.NewOverlayFS(vfs.NewOSFS(), vfs.NewMemMapFS())
}

// NewOSWithEmptyEnv returns the OS-backed bundle with an empty environment, so
// a variable set on the machine running the suite cannot reach the code under
// test. Tests that drive the real filesystem and real subprocesses need it;
// prefer [NewWithOSFS] when only the filesystem has to be real.
//
// Replacing the process environment is a no-op, because parallel tests share
// the process and would lose PATH mid-run.
func NewOSWithEmptyEnv() *venv.Venv {
	return venv.OSVenv().
		WithEnv(map[string]string{}).
		WithReplaceEnviron(func(map[string]string) error { return nil })
}
