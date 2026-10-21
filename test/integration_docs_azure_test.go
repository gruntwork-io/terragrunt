//go:build azure

package test_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureOverviewAzure = "fixtures/docs/02-overview-azure"

	// azureBlobRBACDeniedCode is the error code Azure Storage returns while a
	// newly assigned data-plane role is still propagating.
	azureBlobRBACDeniedCode = "AuthorizationPermissionMismatch"

	// azureDocsMockResourceID is the mock_outputs value the fixture gives the
	// vnet unit, as shown in the overview's mock outputs section.
	azureDocsMockResourceID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/mock-rg"
)

// TestAzureDocsOverview applies and destroys each step of the Azure tab of the
// Getting Started overview, docs/src/content/docs/01-getting-started/02-overview.mdx.
// It does for Azure what TestAWSDocsOverview does for AWS. Each step bootstraps
// its own state resource group and storage account, so the steps run in parallel.
func TestAzureDocsOverview(t *testing.T) {
	t.Parallel()

	steps := []struct {
		name string
		// resourceGroups are the workload resource groups the step creates,
		// without the unique name prefix. In every step but the first, each
		// one pairs with a vnet unit.
		resourceGroups []string
		// single marks the one-unit step, which runs without --all.
		single bool
		// mockPlan marks steps whose vnet unit carries mock_outputs: the docs
		// plan them before anything is applied, so the test does too.
		mockPlan bool
		// mockIDs lists the mock resource group ids the plan output must
		// contain, one per region, so every region's vnet is verified.
		mockIDs []string
	}{
		{name: "step-01-terragrunt.hcl", resourceGroups: []string{"-step-one-rg"}, single: true},
		{name: "step-02-dependencies", resourceGroups: []string{"-step-two-rg"}},
		{name: "step-03-mock-outputs", resourceGroups: []string{"-step-three-rg"}, mockPlan: true,
			mockIDs: []string{azureDocsMockResourceID}},
		{name: "step-04-configuration-hierarchy", resourceGroups: []string{"-step-four-rg"}, mockPlan: true,
			mockIDs: []string{azureDocsMockResourceID}},
		{
			name:           "step-05-exposed-includes",
			resourceGroups: []string{"-step-five-rg-eastus", "-step-five-rg-westus2"},
			mockPlan:       true,
			mockIDs: []string{
				"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/mock-rg-eastus",
				"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/mock-rg-westus2",
			},
		},
	}

	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			t.Parallel()

			env := setupAzureDocsStep(t, step.name, step.resourceGroups)
			rootPath := env.rootPath

			applyCmd := "terragrunt run --all --non-interactive --backend-bootstrap --working-dir " +
				rootPath + " -- apply -auto-approve"
			destroyCmd := "terragrunt run --all --non-interactive --working-dir " +
				rootPath + " -- destroy -auto-approve"

			if step.single {
				applyCmd = "terragrunt run --non-interactive --backend-bootstrap --working-dir " +
					rootPath + " -- apply -auto-approve"
				destroyCmd = "terragrunt destroy -auto-approve --non-interactive --working-dir " + rootPath
			}

			// Clean up the state resource group and every workload resource
			// group in case the destroy below does not finish. These defers
			// are registered before it, so they run after it: workload groups
			// first, then the state group with the storage account.
			defer deleteAzureResourceGroup(t, env.storageAccount, env.stateResourceGroup)

			for _, resourceGroup := range env.workloadResourceGroups {
				defer deleteAzureResourceGroup(t, env.storageAccount, resourceGroup)
			}

			// A failed destroy must not hide the apply error that caused it.
			defer func() {
				_, _, err := helpers.RunTerragruntCommandWithOutput(t, destroyCmd)
				assert.NoError(t, err, "destroying the step's infrastructure")
			}()

			if step.mockPlan {
				planCmd := "terragrunt run --all --non-interactive --backend-bootstrap --working-dir " +
					rootPath + " -- plan"

				stdout, err := runTerragruntUntilRoleAssignmentPropagates(t, planCmd)
				require.NoError(t, err)
				for _, mockID := range step.mockIDs {
					require.GreaterOrEqual(t, strings.Count(stdout, mockID), 1,
						"every region's vnet plan must use its mock resource group id while its rg has no outputs")
				}
			}

			_, err := runTerragruntUntilRoleAssignmentPropagates(t, applyCmd)
			require.NoError(t, err)
		})
	}
}

// azureDocsStepEnv holds what a step run needs to clean up after itself.
type azureDocsStepEnv struct {
	rootPath               string
	storageAccount         string
	stateResourceGroup     string
	workloadResourceGroups []string
}

// setupAzureDocsStep copies one fixture step and points it at a state resource
// group and storage account that do not exist yet. The caller deletes the
// returned resource groups, storage account included, after the step's destroy.
func setupAzureDocsStep(t *testing.T, step string, resourceGroups []string) azureDocsStepEnv {
	t.Helper()

	subscriptionID := requireAzureEnv(t, envAzureSubscriptionID)
	id := strings.ToLower(helpers.UniqueID())
	account := uniqueAzureStorageAccountName("tgdocs")
	stateResourceGroup := "tg-docs-state-" + id

	stepPath := filepath.Join(testFixtureOverviewAzure, step)
	helpers.CleanupTerraformFolder(t, stepPath)
	tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
	rootPath := filepath.Join(tmpEnvPath, stepPath)

	placeholders := map[string]string{
		"__FILL_IN_STORAGE_ACCOUNT__": account,
		"__FILL_IN_RESOURCE_GROUP__":  stateResourceGroup,
		"__FILL_IN_SUBSCRIPTION_ID__": subscriptionID,
		"__FILL_IN_LOCATION__":        azureTestLocation,
		"__FILL_IN_NAME_PREFIX__":     "tg-docs-" + id,
	}

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Lock files end in .hcl too, but hold no placeholders, so leave them alone.
		if d.IsDir() || filepath.Ext(path) != ".hcl" || d.Name() == ".terraform.lock.hcl" {
			return nil
		}

		helpers.CopyAndFillMapPlaceholders(t, path, path, placeholders)

		return nil
	})
	require.NoError(t, err)

	workloadResourceGroups := make([]string, 0, len(resourceGroups))
	for _, suffix := range resourceGroups {
		workloadResourceGroups = append(workloadResourceGroups, placeholders["__FILL_IN_NAME_PREFIX__"]+suffix)
	}

	return azureDocsStepEnv{
		rootPath:               rootPath,
		storageAccount:         account,
		stateResourceGroup:     stateResourceGroup,
		workloadResourceGroups: workloadResourceGroups,
	}
}

// runTerragruntUntilRoleAssignmentPropagates runs cmd and retries it only while
// the Storage Blob Data Contributor role that bootstrap just assigned is still
// propagating. Azure applies role assignments asynchronously, so it can reject
// the first state write after bootstrap for a short while. It returns the
// command's stdout.
func runTerragruntUntilRoleAssignmentPropagates(t *testing.T, cmd string) (string, error) {
	t.Helper()

	deadline := time.Now().Add(azureRBACPropagationTimeout)

	for {
		stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
		if err == nil {
			return stdout, nil
		}

		rbacDenied := strings.Contains(stdout+stderr+err.Error(), azureBlobRBACDeniedCode)
		if !rbacDenied || time.Now().After(deadline) {
			return stdout, err
		}

		t.Logf("role assignment not propagated yet, retrying: %v", errors.Unwrap(err))
		time.Sleep(azureRBACPollInterval)
	}
}
