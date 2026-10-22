//go:build exec && !windows

// Real-git parity tests for GitGetter.
//
// They spawn the git binary against a local [git.Server]. The default build
// pins the command lines through the in-memory exec in git_test.go.

package getter_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecGitGetterClonesThenRefreshesToNewRef(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	srv, err := git.NewServer()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, srv.Close()) })

	require.NoError(t, srv.CommitFile(ctx, "main.tf", []byte("# v1"), "v1"))
	require.NoError(t, srv.Tag(ctx, "v1"))
	require.NoError(t, srv.CommitFile(ctx, "main.tf", []byte("# v2"), "v2"))
	require.NoError(t, srv.Tag(ctx, "v2"))

	url, err := srv.Start(ctx)
	require.NoError(t, err)

	tmp := helpers.TmpDirWOSymlinks(t)
	v := venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return tmp })
	dst := filepath.Join(tmp, "module")
	g := getter.NewGitGetter(v)

	require.NoError(t, download(t, g, "git::"+url+"?ref=v1", dst, getter.ModeDir))

	got, err := vfs.ReadFile(v.FS, filepath.Join(dst, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# v1", string(got))

	require.NoError(t, download(t, g, "git::"+url+"?ref=v2", dst, getter.ModeDir))

	got, err = vfs.ReadFile(v.FS, filepath.Join(dst, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# v2", string(got), "the existing checkout must move to the new ref")
}

func TestExecGitGetterClonesAtDepthOne(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	srv, err := git.NewServer()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, srv.Close()) })

	require.NoError(t, srv.CommitEmptyChain(ctx, 3, "history"))
	require.NoError(t, srv.CommitFile(ctx, "main.tf", []byte("# tip"), "tip"))

	url, err := srv.Start(ctx)
	require.NoError(t, err)

	tmp := helpers.TmpDirWOSymlinks(t)
	v := venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return tmp })
	dst := filepath.Join(tmp, "module")

	require.NoError(t, download(t, getter.NewGitGetter(v), "git::"+url+"?depth=1", dst, getter.ModeDir))

	runner, err := git.NewGitRunner(v)
	require.NoError(t, err)

	shallow, err := runner.WithWorkDir(dst).IsShallow(ctx)
	require.NoError(t, err)
	assert.True(t, shallow, "depth=1 must produce a shallow clone")
}
