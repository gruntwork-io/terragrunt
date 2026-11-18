//go:build gcp

package test_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/require"
)

func TestGCPExampleLiveStacks(t *testing.T) {
	t.Parallel()

	project := venv.OSVenv().Env["GOOGLE_CLOUD_PROJECT"]
	require.NotEmpty(t, project, "set GOOGLE_CLOUD_PROJECT to run the GCP integration tests")

	prefix := "tglive" + strings.ToLower(helpers.UniqueID())
	env := []string{
		"EX_APP_PREFIX=" + prefix,
		"EX_NON_PROD_PROJECT_ID=" + project,
	}

	tmpDir := helpers.TmpDirWOSymlinks(t)
	repoDir := filepath.Join(tmpDir, "live-gcp-example")

	helpers.ExecWithTestLogger(
		t,
		tmpDir,
		"git",
		"clone",
		"https://github.com/gruntwork-io/terragrunt-infrastructure-live-gcp-example.git",
		repoDir,
	)
	helpers.ExecWithTestLogger(t, repoDir, "git", "checkout", "main")

	helpers.ExecWithTestLogger(t, repoDir, "mise", "trust")
	helpers.ExecWithTestLogger(t, repoDir, "mise", "install")

	stackDir := filepath.Join(repoDir, "non-prod", "us-east1", "app-foundation")

	defer helpers.ExecWithTestLogger(t, stackDir, "env",
		gcpLiveExampleTerragrunt(env, "run", "--all", "--non-interactive", "--filter", "bucket", "destroy")...)

	helpers.ExecWithTestLogger(t, stackDir, "env",
		gcpLiveExampleTerragrunt(
			env, "run", "--all", "--non-interactive", "--backend-bootstrap", "--filter", "bucket", "apply")...)

	validateGCSBucketExistsAndIsLabeled(t, "us-east1", prefix+"-app-dev-data", nil)
}

// gcpLiveExampleTerragrunt returns the arguments of an `env` command that runs the example's pinned terragrunt.
func gcpLiveExampleTerragrunt(env []string, args ...string) []string {
	return slices.Concat(env, []string{"mise", "x", "terragrunt", "--", "terragrunt"}, args)
}
