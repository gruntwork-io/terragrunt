package module_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

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
		CloneURL:      "git::https://example.invalid/acme/terraform-aws-modules.git",
		Path:          filepath.Join(tmpDir, "repo"),
		AllowCAS:      true,
		CASCloneDepth: 1,
	})
	require.Error(t, err)
}
