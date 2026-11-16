//go:build azure

package test_test

import (
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	liveExampleStartTimeout = 3 * time.Minute
	liveExamplePollInterval = 5 * time.Second
)

func TestAzureExampleLiveStacks(t *testing.T) {
	t.Parallel()

	subscriptionID := requireAzureEnv(t, envAzureSubscriptionID)

	prefix := "tglive" + strings.ToLower(helpers.UniqueID())
	stateAccount := prefix + "nonprodtf"
	env := []string{
		"EX_APP_PREFIX=" + prefix,
		"EX_NON_PROD_SUBSCRIPTION_ID=" + subscriptionID,
	}

	tmpDir := helpers.TmpDirWOSymlinks(t)
	repoDir := filepath.Join(tmpDir, "live-azure-example")

	helpers.ExecWithTestLogger(
		t,
		tmpDir,
		"git",
		"clone",
		"https://github.com/gruntwork-io/terragrunt-infrastructure-live-azure-example.git",
		repoDir,
	)
	helpers.ExecWithTestLogger(t, repoDir, "git", "checkout", "main")

	helpers.ExecWithTestLogger(t, repoDir, "mise", "trust")
	helpers.ExecWithTestLogger(t, repoDir, "mise", "install")

	stackDir := filepath.Join(repoDir, "non-prod", "eastus", "hello-world")

	defer deleteAzureResourceGroup(t, stateAccount, prefix+"-tfstate")
	defer deleteAzureResourceGroup(t, stateAccount, prefix+"-hello-dev-rg")

	defer helpers.ExecWithTestLogger(t, stackDir, "env",
		liveExampleTerragrunt(env, "run", "--all", "--non-interactive", "destroy")...)

	applyAzureLiveExample(t, stackDir,
		liveExampleTerragrunt(env, "run", "--all", "--non-interactive", "--backend-bootstrap", "apply"))

	url, _ := helpers.ExecAndCaptureOutput(t, stackDir, "env",
		liveExampleTerragrunt(env, "stack", "output", "--raw", "website.url")...)

	requireHelloWorld(t, strings.TrimSpace(url), "Hello, world!")
}

// applyAzureLiveExample retries the apply while the blob data role from bootstrap propagates.
func applyAzureLiveExample(t *testing.T, dir string, envArgs []string) {
	t.Helper()

	attempts := int(azureRBACPropagationTimeout / azureRBACPollInterval)

	for range attempts {
		cmd := exec.CommandContext(t.Context(), "env", envArgs...)
		cmd.Dir = dir

		output, err := cmd.CombinedOutput()
		t.Logf("%s", output)

		if err == nil {
			return
		}

		require.Contains(t, string(output), azureBlobRBACDeniedCode, "apply failed: %v", err)

		time.Sleep(azureRBACPollInterval)
	}

	require.FailNow(t, "the blob data role did not propagate", "gave up after %d applies", attempts)
}

// liveExampleTerragrunt returns the arguments of an `env` command that runs the example's pinned terragrunt.
func liveExampleTerragrunt(env []string, args ...string) []string {
	return slices.Concat(env, []string{"mise", "x", "terragrunt", "--", "terragrunt"}, args)
}

// requireHelloWorld polls the HTTPS url until it answers 200 with a body that contains want.
func requireHelloWorld(t *testing.T, url, want string) {
	t.Helper()

	require.True(t, strings.HasPrefix(url, "https://"), "expected an HTTPS URL, got %q", url)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
		require.NoError(c, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(c, err)

		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(c, err)

		assert.Equal(c, http.StatusOK, resp.StatusCode)
		assert.Contains(c, string(body), want)
	}, liveExampleStartTimeout, liveExamplePollInterval)
}
