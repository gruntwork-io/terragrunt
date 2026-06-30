package cas_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/require"
)

// TestVenvRequireFS pins the FS contract: the zero Venv panics with the
// sentinel, a populated Venv passes.
func TestVenvRequireFS(t *testing.T) {
	t.Parallel()

	require.PanicsWithValue(t, cas.ErrVenvFSUnset, func() {
		cas.Venv{}.RequireFS()
	})

	require.NotPanics(t, func() {
		cas.Venv{FS: vfs.NewOSFS()}.RequireFS()
	})
}

// TestVenvRequireGit pins the Git contract. A Venv with FS but no Git
// must still panic; only a populated Git satisfies the check.
func TestVenvRequireGit(t *testing.T) {
	t.Parallel()

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	require.PanicsWithValue(t, cas.ErrVenvGitUnset, func() {
		cas.Venv{FS: vfs.NewOSFS()}.RequireGit()
	})

	require.NotPanics(t, func() {
		cas.Venv{FS: vfs.NewOSFS(), Git: runner}.RequireGit()
	})
}
