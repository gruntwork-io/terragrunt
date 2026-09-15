//go:build exec

package module_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestExecResolveLatestTagOfflineServesRecordedTags pins that an offline
// catalog resolves the latest release tag an earlier online catalog listed,
// without running a git command that reaches the remote.
func TestExecResolveLatestTagOfflineServesRecordedTags(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	l := logger.CreateLogger()

	v := venvtest.NewOSWithEmptyEnv().
		WithUserCacheDir(func() (string, error) { return filepath.Join(tmpDir, "cache"), nil })

	remote := newTaggedRemote(t, v, tmpDir, "v1.0.0", "v1.2.0", "v2.0.0-rc1")
	cloneURL := "git::file://" + filepath.ToSlash(remote)

	online, err := module.NewRepo(t.Context(), l, v, &module.RepoOpts{
		CloneURL:      cloneURL,
		Path:          filepath.Join(tmpDir, "online"),
		AllowCAS:      true,
		CASCloneDepth: 1,
		CASProbeCache: true,
	})
	require.NoError(t, err)

	online.ResolveLatestTag(t.Context(), l, v)
	require.Equal(t, "v1.2.0", online.LatestTag)

	recorder := &gitArgsRecorder{Exec: v.Exec}
	offlineV := v.WithExec(recorder)

	offline, err := module.NewRepo(t.Context(), l, offlineV, &module.RepoOpts{
		CloneURL:      cloneURL,
		Path:          filepath.Join(tmpDir, "offline"),
		AllowCAS:      true,
		CASCloneDepth: 1,
		CASOffline:    true,
		CASProbeCache: true,
	})
	require.NoError(t, err)

	offline.ResolveLatestTag(t.Context(), l, offlineV)

	assert.Equal(t, "v1.2.0", offline.LatestTag)

	for _, args := range recorder.recorded() {
		assert.False(t, slices.ContainsFunc(args, isRemoteGitCommand), "git %v reaches the remote", args)
	}
}

// newTaggedRemote creates a bare repository under dir holding one commit with
// each of tags, and returns its path.
func newTaggedRemote(t *testing.T, v *venv.Venv, dir string, tags ...string) string {
	t.Helper()

	work := filepath.Join(dir, "work")
	remote := filepath.Join(dir, "remote.git")

	require.NoError(t, os.MkdirAll(filepath.Join(work, "modules", "vpc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(work, "modules", "vpc", "main.tf"), []byte("variable \"cidr\" {}\n"), 0o600))

	runGit(t, v, work, "init", "--quiet", "--initial-branch", "main")
	runGit(t, v, work, "add", "--all")
	runGit(t, v, work, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "--quiet", "--message", "init")

	for _, tag := range tags {
		runGit(t, v, work, "tag", tag)
	}

	runGit(t, v, dir, "clone", "--quiet", "--bare", work, remote)

	return remote
}

// runGit runs git with args in dir through v's exec and fails the test when it
// exits non-zero.
func runGit(t *testing.T, v *venv.Venv, dir string, args ...string) {
	t.Helper()

	var stderr bytes.Buffer

	cmd := v.Exec.Command(t.Context(), "git", args...)
	cmd.SetDir(dir)
	cmd.SetStderr(&stderr)

	require.NoError(t, cmd.Run(), stderr.String())
}

// gitArgsRecorder records the arguments of every command prepared through it.
type gitArgsRecorder struct {
	vexec.Exec
	args [][]string
	mu   sync.Mutex
}

// Command records args and prepares the command through the wrapped Exec.
func (r *gitArgsRecorder) Command(ctx context.Context, name string, args ...string) vexec.Cmd {
	r.mu.Lock()
	r.args = append(r.args, args)
	r.mu.Unlock()

	return r.Exec.Command(ctx, name, args...)
}

// recorded returns the arguments recorded so far.
func (r *gitArgsRecorder) recorded() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.args)
}
