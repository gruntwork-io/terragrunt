//go:build gcp

package test_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/require"
)

const (
	gcpLiveExampleRepo = "https://github.com/gruntwork-io/terragrunt-infrastructure-live-gcp-example.git"
	gcpLiveExampleRef  = "c44f1c8dc17ab96401700c13de2a0a84718126df"
)

func TestGCPExampleLiveStacks(t *testing.T) {
	t.Parallel()

	project := venv.OSVenv().Env["GOOGLE_CLOUD_PROJECT"]
	require.NotEmpty(t, project, "GOOGLE_CLOUD_PROJECT must name the project the live example runs in")

	prefix := "tglive" + strings.ToLower(helpers.UniqueID())
	env := []string{
		"EX_APP_PREFIX=" + prefix,
		"EX_BUCKET_PREFIX=" + prefix + "-",
		"EX_NON_PROD_PROJECT_ID=" + project,
	}

	repoDir := cloneLiveExample(t, gcpLiveExampleRepo, gcpLiveExampleRef)
	stackDir := filepath.Join(repoDir, "non-prod", "us-east1", "hello-world")

	defer deleteGCSBucket(t, prefix+"-"+project+"-non-prod-tfstate")

	defer helpers.ExecWithTestLogger(t, stackDir, "env",
		liveExampleTerragrunt(env, "run", "--all", "--non-interactive", "destroy")...)

	helpers.ExecWithTestLogger(t, stackDir, "env",
		liveExampleTerragrunt(env, "run", "--all", "--non-interactive", "--backend-bootstrap", "apply")...)

	url, _ := helpers.ExecAndCaptureOutput(t, stackDir, "env",
		liveExampleTerragrunt(env, "stack", "output", "--raw", "service.url")...)

	requireHelloWorld(t, strings.TrimSpace(url), "Hello, world!")
}
