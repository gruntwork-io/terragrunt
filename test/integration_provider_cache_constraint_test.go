package test_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureProviderCacheWeakConstraint = "fixtures/provider-cache/weak-constraint"
)

// TestTerragruntProviderCacheWeakConstraint tests that provider cache preserves
// module constraints instead of pinning exact versions in .terraform.lock.hcl files.
// Reproduces and validates the fix for GitHub issue #4512.
//
//nolint:paralleltest,tparallel
func TestTerragruntProviderCacheWeakConstraint(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureProviderCacheWeakConstraint)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureProviderCacheWeakConstraint)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureProviderCacheWeakConstraint)
	appPath := filepath.Join(rootPath, "app")

	providerCacheDir := t.TempDir()

	t.Run("initial_setup_preserves_module_constraints", func(t *testing.T) {
		helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt init --provider-cache --provider-cache-dir %s --log-level trace --non-interactive --working-dir %s", providerCacheDir, appPath))

		constraintsValue := extractConstraintsFromLockFile(t, appPath, "cloudflare/cloudflare")

		expectedConstraints := "~> 4.0.0"
		assert.Equal(t, expectedConstraints, constraintsValue, "Initial lock file should preserve module's required_providers constraints")
	})

	t.Run("upgrade_updates_constraints_to_match_module", func(t *testing.T) {
		// Update the main.tf file to change cloudflare version constraint from "~> 4.0" to "~> 4.40"
		mainTfPath := filepath.Join(appPath, "main.tf")
		originalContent, err := os.ReadFile(mainTfPath)
		require.NoError(t, err)

		// Replace the version constraint
		updatedContent := strings.ReplaceAll(string(originalContent), `version = "~> 4.0"`, `version = "~> 4.40"`)
		require.NotEqual(t, string(originalContent), updatedContent, "Content should be different after replacement")

		err = os.WriteFile(mainTfPath, []byte(updatedContent), 0644)
		require.NoError(t, err)

		lockFilePreInit, err := os.ReadFile(filepath.Join(appPath, ".terraform.lock.hcl"))
		require.NoError(t, err)

		// Run terragrunt init and check that the lock file isn't updated
		helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt init --provider-cache --provider-cache-dir %s --log-level trace --non-interactive --working-dir %s", providerCacheDir, appPath))
		lockFilePostInit, err := os.ReadFile(filepath.Join(appPath, ".terraform.lock.hcl"))
		require.NoError(t, err)
		assert.Equal(t, string(lockFilePreInit), string(lockFilePostInit), "Lock file should not be updated")

		// Run terragrunt init -upgrade to update the lock file
		helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt init -upgrade --provider-cache --provider-cache-dir %s --log-level trace --non-interactive --working-dir %s", providerCacheDir, appPath))

		lockFilePostUpgrade, err := os.ReadFile(filepath.Join(appPath, ".terraform.lock.hcl"))
		require.NoError(t, err)
		assert.NotEqual(t, string(lockFilePostInit), string(lockFilePostUpgrade), "Lock file should be updated")

		// Verify the lock file constraints are updated to match the module
		constraintsValue := extractConstraintsFromLockFile(t, appPath, "cloudflare/cloudflare")

		expectedConstraints := "~> 4.40.0"
		assert.Equal(t, expectedConstraints, constraintsValue, "Constraints should be updated to match the module's required_providers")
	})

	t.Run("fresh_start_uses_module_constraints", func(t *testing.T) {
		// Delete the lock file
		lockfilePath := filepath.Join(appPath, ".terraform.lock.hcl")
		err := os.Remove(lockfilePath)
		require.NoError(t, err)

		// Also clean up .terraform directory to ensure fresh start
		terraformDir := filepath.Join(appPath, ".terraform")
		if util.FileExists(terraformDir) {
			err = os.RemoveAll(terraformDir)
			require.NoError(t, err)
		}

		helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt init --provider-cache --provider-cache-dir %s --log-level trace --non-interactive --working-dir %s", providerCacheDir, appPath))

		constraintsValue := extractConstraintsFromLockFile(t, appPath, "cloudflare/cloudflare")

		expectedConstraints := "~> 4.40.0"
		assert.Equal(t, expectedConstraints, constraintsValue, "Fresh lock file should use module's required_providers constraints")
	})
}

// Helper function to extract constraints value from lock file
func extractConstraintsFromLockFile(t *testing.T, appPath string, providerName string) string {
	t.Helper()

	lockfilePath := filepath.Join(appPath, ".terraform.lock.hcl")
	require.True(t, util.FileExists(lockfilePath), "Lock file should exist")

	// Read and parse the lock file
	lockfileContent, err := os.ReadFile(lockfilePath)
	require.NoError(t, err)

	lockfile, diags := hclwrite.ParseConfig(lockfileContent, lockfilePath, hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "Lock file should be valid HCL")

	// Find the provider block (handle both short and full provider names)
	var providerBlock *hclwrite.Block
	if strings.Contains(providerName, "/") {
		// Full name like "cloudflare/cloudflare"
		providerBlock = lockfile.Body().FirstMatchingBlock("provider", []string{"registry.terraform.io/" + providerName})
		if providerBlock == nil {
			// Try OpenTofu registry as well
			providerBlock = lockfile.Body().FirstMatchingBlock("provider", []string{"registry.opentofu.org/" + providerName})
		}
	} else {
		// Short name - search for matching block
		for _, block := range lockfile.Body().Blocks() {
			if block.Type() == "provider" && len(block.Labels()) > 0 {
				if strings.Contains(block.Labels()[0], providerName) {
					providerBlock = block
					break
				}
			}
		}
	}

	require.NotNil(t, providerBlock, "Provider block should exist in lock file")

	// Get the constraints attribute
	constraintsAttr := providerBlock.Body().GetAttribute("constraints")
	require.NotNil(t, constraintsAttr, "Constraints attribute should exist")

	constraintsValue := strings.Trim(string(constraintsAttr.Expr().BuildTokens(nil).Bytes()), ` "`)

	return constraintsValue
}
