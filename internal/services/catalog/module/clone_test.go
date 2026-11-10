package module_test

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/venv"
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

// TestCloneProgress checks that a slow clone is drawn with a reporter.
func TestCloneProgress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		reported bool
	}{
		{name: "reporter on the context", reported: true},
		{name: "plain context"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
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

				term := new(bytes.Buffer)

				ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
				defer cancel()

				if tc.reported {
					ctx = spinner.ContextWithReporter(ctx, drawingReporter(term))
				}

				_, err := module.NewRepo(ctx, logger.CreateLogger(), v, &module.RepoOpts{
					CloneURL:      absentRepoURL(tmpDir),
					Path:          filepath.Join(tmpDir, "slow-repo"),
					AllowCAS:      true,
					CASCloneDepth: 1,
				})
				require.Error(t, err)

				if tc.reported {
					assert.NotEmpty(t, term.String())
				}
			})
		})
	}
}

// drawingReporter returns a reporter that draws its progress line on term from the start.
func drawingReporter(term io.Writer) *spinner.Reporter {
	v := venv.MemVenv().WithErrWriter(term).WithEnv(map[string]string{"TERM": "xterm"}).WithGOOS("linux")
	v.Terminal.StderrIsTTY = func() bool { return true }
	v.Terminal.StderrHandlesEscapes = func() bool { return true }
	v.Terminal.ErrWidth = func() int { return 120 }

	return spinner.New(v, spinner.Options{LogsForHumans: true})
}
