//go:build azure

package test_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/require"
)

const (
	azureLiveExampleRepo = "https://github.com/gruntwork-io/terragrunt-infrastructure-live-azure-example.git"
	azureLiveExampleRef  = "4a1ca91dd49d2bf3252652a64da3900d220e06be"
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

	repoDir := cloneLiveExample(t, azureLiveExampleRepo, azureLiveExampleRef)
	stackDir := filepath.Join(repoDir, "non-prod", "eastus", "hello-world")

	defer deleteAzureResourceGroup(t, stateAccount, prefix+"-tfstate")
	defer deleteAzureResourceGroup(t, stateAccount, prefix+"-hello-dev-rg")

	defer helpers.ExecWithTestLogger(t, stackDir, "env",
		liveExampleTerragrunt(env, "run", "--all", "--non-interactive", "destroy")...)

	applyAzureLiveExample(t, stackDir,
		liveExampleTerragrunt(env, "run", "--all", "--non-interactive", "--backend-bootstrap", "apply"))

	url, _ := helpers.ExecAndCaptureOutput(t, stackDir, "env",
		liveExampleTerragrunt(env, "stack", "output", "--raw", "service.url")...)

	requireHelloWorld(t, strings.TrimSpace(url), "Welcome to Azure Container Instances!")
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
