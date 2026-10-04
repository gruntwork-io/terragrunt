package module_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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

// TestCloneReportsProgressAndHidesCredentials pins what a slow catalog clone
// logs: a keepalive line when the run carries a progress reporter and none
// when it does not, and never the credentials of the clone URL.
func TestCloneReportsProgressAndHidesCredentials(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		withReporter  bool
		wantKeepalive bool
	}{
		{
			name:          "experiment on",
			withReporter:  true,
			wantKeepalive: true,
		},
		{
			name: "experiment off",
		},
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

				logs := new(bytes.Buffer)
				l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(util.NewSyncWriter(logs)))

				ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
				defer cancel()

				if tc.withReporter {
					ctx = spinner.ContextWithReporter(ctx, spinner.New(spinner.Options{}))
				}

				absent := strings.TrimPrefix(filepath.ToSlash(filepath.Join(tmpDir, "absent.git")), "/")

				_, err := module.NewRepo(ctx, l, v, &module.RepoOpts{
					CloneURL:      "git::file://alice:s3cret@localhost/" + absent + "?access_token=t0ken&access_token=t0ken2&note=a%20b",
					Path:          filepath.Join(tmpDir, "slow-repo"),
					AllowCAS:      true,
					CASCloneDepth: 1,
				})
				require.Error(t, err)

				assert.Contains(t, err.Error(), "absent.git")
				assert.NotContains(t, err.Error(), "s3cret")
				assert.NotContains(t, err.Error(), "t0ken")
				assert.NotContains(t, err.Error(), "REDACTED2", "a value that starts with another one is hidden whole")
				assert.NotContains(t, err.Error(), "note=a")
				assert.Contains(t, logs.String(), "Cloning repository")
				assert.Contains(t, logs.String(), "file://localhost/"+absent)
				assert.NotContains(t, logs.String(), "s3cret")
				assert.NotContains(t, logs.String(), "t0ken")
				assert.Equal(t, tc.wantKeepalive, strings.Contains(logs.String(), "(30s elapsed)"))
			})
		})
	}
}
