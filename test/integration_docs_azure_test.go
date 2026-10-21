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
	"github.com/stretchr/testify/require"
)

const (
	testFixtureOverviewAzure = "fixtures/docs/02-overview-azure"

	// azureBlobRBACDeniedCode is the Azure Storage error code returned while a
	// freshly assigned data-plane role has not propagated yet.
	azureBlobRBACDeniedCode = "AuthorizationPermissionMismatch"
)

// TestAzureDocsOverview applies and destroys each step of the Azure tab of the
// Getting Started overview (docs/src/content/docs/01-getting-started/02-overview.mdx),
// mirroring TestAWSDocsOverview. Every step bootstraps its own state resource
// group and storage account so steps can run in parallel.
func TestAzureDocsOverview(t *testing.T) {
	t.Parallel()

	steps := []struct {
		name   string
		single bool
	}{
		{name: "step-01-terragrunt.hcl", single: true},
		{name: "step-02-dependencies"},
		{name: "step-03-mock-outputs"},
		{name: "step-04-configuration-hierarchy"},
		{name: "step-05-exposed-includes"},
	}

	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			t.Parallel()

			rootPath := setupAzureDocsStep(t, step.name)

			applyCmd := "terragrunt run --all --non-interactive --backend-bootstrap --working-dir " +
				rootPath + " -- apply -auto-approve"
			destroyCmd := "terragrunt run --all --non-interactive --working-dir " +
				rootPath + " -- destroy -auto-approve"

			if step.single {
				applyCmd = "terragrunt run --non-interactive --backend-bootstrap --working-dir " +
					rootPath + " -- apply -auto-approve"
				destroyCmd = "terragrunt destroy -auto-approve --non-interactive --working-dir " + rootPath
			}

			defer func() {
				_, _, err := helpers.RunTerragruntCommandWithOutput(t, destroyCmd)
				require.NoError(t, err)
			}()

			require.NoError(t, runTerragruntUntilRoleAssignmentPropagates(t, applyCmd))
		})
	}
}

// setupAzureDocsStep copies one fixture step, points it at a state resource
// group and storage account that do not exist yet, and registers cleanup of
// that resource group, which also reclaims the storage account inside it.
func setupAzureDocsStep(t *testing.T, step string) string {
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

		// Provider lock files share the extension but hold no placeholders, and
		// rewriting them read-only would stop OpenTofu/Terraform updating them.
		if d.IsDir() || filepath.Ext(path) != ".hcl" || d.Name() == ".terraform.lock.hcl" {
			return nil
		}

		helpers.CopyAndFillMapPlaceholders(t, path, path, placeholders)

		return nil
	})
	require.NoError(t, err)

	t.Cleanup(func() { deleteAzureResourceGroup(t, account, stateResourceGroup) })

	return rootPath
}

// runTerragruntUntilRoleAssignmentPropagates runs cmd, retrying only while the
// Storage Blob Data Contributor role that bootstrap just assigned is still
// propagating. Azure applies role assignments asynchronously, so the first
// state write after bootstrap can be rejected for a short while.
func runTerragruntUntilRoleAssignmentPropagates(t *testing.T, cmd string) error {
	t.Helper()

	deadline := time.Now().Add(azureRBACPropagationTimeout)

	for {
		stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
		if err == nil {
			return nil
		}

		rbacDenied := strings.Contains(stdout+stderr+err.Error(), azureBlobRBACDeniedCode)
		if !rbacDenied || time.Now().After(deadline) {
			return err
		}

		t.Logf("role assignment not propagated yet, retrying: %v", errors.Unwrap(err))
		time.Sleep(azureRBACPollInterval)
	}
}
