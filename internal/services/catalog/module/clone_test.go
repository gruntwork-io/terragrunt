package module_test

import (
	"context"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// absentRepoURL names a git remote that does not exist. The scheme is file://
// so the plain git getter the download falls back to, which spawns git itself
// rather than going through the venv, stays off the network too.
func absentRepoURL(dir string) string {
	return "git::file://" + filepath.ToSlash(filepath.Join(dir, "absent.git"))
}

// TestCloneReportsFailingGitRemote pins that a catalog clone whose git
// commands all fail surfaces the failure as an error. The CAS getters log the
// probe failure before falling back to a content hash, so a getter client
// built without a logger used to take the whole process down here instead
// (gruntwork-io/terragrunt#6869).
func TestCloneReportsFailingGitRemote(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	failGit := func(context.Context, vexec.Invocation) vexec.Result {
		return vexec.Result{ExitCode: 128, Stderr: []byte("fatal: repository not found")}
	}

	v := venvtest.NewWithOSFS().
		WithExec(vexec.NewMemExec(failGit)).
		WithUserCacheDir(func() (string, error) { return filepath.Join(tmpDir, "cache"), nil })

	_, err := module.NewRepo(t.Context(), logger.CreateLogger(), v, &module.RepoOpts{
		CloneURL:      absentRepoURL(tmpDir),
		Path:          filepath.Join(tmpDir, "repo"),
		AllowCAS:      true,
		CASCloneDepth: 1,
	})
	require.Error(t, err)
}

// TestCloneReportsAGitRemoteThatNeverAnswers pins the same reporting for a
// clone the caller's deadline cuts off. A cancelled git command fails the CAS
// source probe the way an outright error does, so both shapes travel the path
// that used to panic.
func TestCloneReportsAGitRemoteThatNeverAnswers(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	synctest.Test(t, func(t *testing.T) {
		hangGit := func(ctx context.Context, _ vexec.Invocation) vexec.Result {
			<-ctx.Done()

			return vexec.Result{Err: ctx.Err()}
		}

		v := venvtest.NewWithOSFS().
			WithExec(vexec.NewMemExec(hangGit)).
			WithUserCacheDir(func() (string, error) { return filepath.Join(tmpDir, "cache"), nil })

		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()

		_, err := module.NewRepo(ctx, logger.CreateLogger(), v, &module.RepoOpts{
			CloneURL:      absentRepoURL(tmpDir),
			Path:          filepath.Join(tmpDir, "slow-repo"),
			AllowCAS:      true,
			CASCloneDepth: 1,
		})
		require.Error(t, err)
	})
}
