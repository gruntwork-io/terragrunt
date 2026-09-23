package module_test

import (
	"context"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestCloneOfflineFailsOnSourceMissingFromStore pins that an offline catalog
// clone of a source the CAS store lacks fails with the offline-miss error, and
// that no git command it runs through the venv reaches for the remote. The
// plain git getter spawns git outside the venv, so the error is what shows it
// never ran: its failure would replace the offline miss.
func TestCloneOfflineFailsOnSourceMissingFromStore(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	var (
		mu      sync.Mutex
		gitArgs [][]string
	)

	recordGit := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		mu.Lock()
		defer mu.Unlock()

		gitArgs = append(gitArgs, inv.Args)

		return vexec.Result{ExitCode: 128, Stderr: []byte("fatal: repository not found")}
	}

	v := venvtest.NewWithOSFS().
		WithExec(vexec.NewMemExec(recordGit)).
		WithUserCacheDir(func() (string, error) { return filepath.Join(tmpDir, "cache"), nil })

	_, err := module.NewRepo(t.Context(), logger.CreateLogger(), v, &module.RepoOpts{
		CloneURL:      absentRepoURL(tmpDir),
		Path:          filepath.Join(tmpDir, "repo"),
		AllowCAS:      true,
		CASCloneDepth: 1,
		CASOffline:    true,
		CASProbeCache: true,
	})
	require.ErrorIs(t, err, cas.ErrCASOffline)

	mu.Lock()
	defer mu.Unlock()

	for _, args := range gitArgs {
		assert.False(t, slices.ContainsFunc(args, isRemoteGitCommand), "git %v reaches the remote", args)
	}
}

func isRemoteGitCommand(arg string) bool {
	return arg == "clone" || arg == "fetch" || arg == "ls-remote"
}
