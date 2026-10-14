//go:build tf

package test_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureProviderCacheWeakConstraint = "fixtures/provider-cache/weak-constraint"
)

// TestTFTerragruntProviderCacheWeakConstraint tests that provider cache preserves
// module constraints instead of pinning exact versions in .terraform.lock.hcl files.
// Reproduces and validates the fix for GitHub issue #4512.
//
//nolint:paralleltest,tparallel // the subtests build on each other in one working dir
func TestTFTerragruntProviderCacheWeakConstraint(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureProviderCacheWeakConstraint)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureProviderCacheWeakConstraint)
	rootPath := filepath.Join(tmpEnvPath, testFixtureProviderCacheWeakConstraint)
	appPath := filepath.Join(rootPath, "app")

	providerCacheDir := helpers.TmpDirWOSymlinks(t)

	t.Run("initial_setup_preserves_module_constraints", func(t *testing.T) {
		helpers.RunTerragrunt(
			t,
			fmt.Sprintf(
				"terragrunt init --provider-cache --provider-cache-dir %s --non-interactive --working-dir %s",
				providerCacheDir,
				appPath,
			),
		)

		constraintsValue := extractConstraintsFromLockFile(t, appPath, "cloudflare/cloudflare")

		expectedConstraints := "~> 4.40.0"
		assert.Equal(
			t,
			expectedConstraints,
			constraintsValue,
			"Initial lock file should preserve module's required_providers constraints",
		)
	})

	t.Run("upgrade_updates_constraints_to_match_module", func(t *testing.T) {
		// Update the main.tf file to change cloudflare version constraint from "~> 4.40.0" to "~> 4.40"
		mainTfPath := filepath.Join(appPath, "main.tf")
		originalContent, err := os.ReadFile(mainTfPath)
		require.NoError(t, err)

		// Replace the version constraint
		updatedContent := strings.ReplaceAll(
			string(originalContent),
			`version = "~> 4.40.0"`,
			`version = "~> 4.40"`,
		)
		require.NotEqual(
			t,
			string(originalContent),
			updatedContent,
			"Content should be different after replacement",
		)

		err = os.WriteFile(mainTfPath, []byte(updatedContent), 0644)
		require.NoError(t, err)

		lockFilePreInit, err := os.ReadFile(filepath.Join(appPath, ".terraform.lock.hcl"))
		require.NoError(t, err)

		// Run terragrunt init and check that the lock file isn't updated
		helpers.RunTerragrunt(
			t,
			fmt.Sprintf(
				"terragrunt init --provider-cache --provider-cache-dir %s --non-interactive --working-dir %s",
				providerCacheDir,
				appPath,
			),
		)
		lockFilePostInit, err := os.ReadFile(filepath.Join(appPath, ".terraform.lock.hcl"))
		require.NoError(t, err)
		assert.Equal(
			t,
			string(lockFilePreInit),
			string(lockFilePostInit),
			"Lock file should not be updated",
		)

		// Run terragrunt init -upgrade to update the lock file
		helpers.RunTerragrunt(
			t,
			fmt.Sprintf(
				"terragrunt init -upgrade --provider-cache --provider-cache-dir %s --non-interactive --working-dir %s",
				providerCacheDir,
				appPath,
			),
		)

		lockFilePostUpgrade, err := os.ReadFile(filepath.Join(appPath, ".terraform.lock.hcl"))
		require.NoError(t, err)
		assert.NotEqual(
			t,
			string(lockFilePostInit),
			string(lockFilePostUpgrade),
			"Lock file should be updated",
		)

		// Verify the lock file constraints are updated to match the module
		constraintsValue := extractConstraintsFromLockFile(t, appPath, "cloudflare/cloudflare")

		expectedConstraints := "~> 4.40"
		assert.Equal(
			t,
			expectedConstraints,
			constraintsValue,
			"Constraints should be updated to match the module's required_providers",
		)
	})

	t.Run("fresh_start_uses_module_constraints", func(t *testing.T) {
		// Delete the lock file
		lockfilePath := filepath.Join(appPath, ".terraform.lock.hcl")
		err := os.Remove(lockfilePath)
		require.NoError(t, err)

		// Also clean up .terraform directory to ensure fresh start
		terraformDir := filepath.Join(appPath, ".terraform")
		if vfs.Exists(vfs.NewOSFS(), terraformDir) {
			err = os.RemoveAll(terraformDir)
			require.NoError(t, err)
		}

		helpers.RunTerragrunt(
			t,
			fmt.Sprintf(
				"terragrunt init --provider-cache --provider-cache-dir %s --non-interactive --working-dir %s",
				providerCacheDir,
				appPath,
			),
		)

		constraintsValue := extractConstraintsFromLockFile(t, appPath, "cloudflare/cloudflare")

		expectedConstraints := "~> 4.40"
		assert.Equal(
			t,
			expectedConstraints,
			constraintsValue,
			"Fresh lock file should use module's required_providers constraints",
		)
	})
}

// TestTFProviderCacheLockConstraintsMatchTofu pins that the provider cache
// records each `constraints` value the way tofu records it, and that tofu then
// installs from that lock file without changing it.
func TestTFProviderCacheLockConstraintsMatchTofu(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		version string
	}{
		{name: "pessimistic with two parts", version: "~> 3.0"},
		{name: "pessimistic with one part", version: "~> 3"},
		{name: "pessimistic with a nonzero minor", version: "~> 3.1"},
		{name: "pessimistic with three parts", version: "~> 3.2.0"},
		{name: "pessimistic without a space", version: "~>3.0"},
		{name: "lower bound with two parts", version: ">= 3.0"},
		{name: "lower bound without a space", version: ">=3.0"},
		{name: "range", version: ">= 3.0, < 4.0"},
		{name: "range with the upper bound first", version: "< 4.0, >= 3.0"},
		{name: "three terms out of order", version: "!= 3.2.0, < 4, ~> 3.0"},
		{name: "repeated lower bound", version: ">= 3.0, >= 3.0.0"},
		{name: "repeated pessimistic", version: "~> 3, ~> 3.0"},
		{name: "repeated exact", version: "= 3.2.3, 3.2.3"},
		{name: "pessimistic precisions sharing a version", version: "~> 3.2, ~> 3.2.0"},
		{name: "lower bounds sharing a version", version: ">= 3.1, > 3.1"},
		{name: "upper bounds sharing a version", version: "< 3.2.3, <= 3.2.3"},
		{name: "exclusion sharing a version with a lower bound", version: "!= 3.2.0, >= 3.2.0"},
		{
			name:    "exact sharing a version with other operators",
			version: "<= 3.2.3, ~> 3.2.3, 3.2.3, >= 3.2.3",
		},
		{name: "pessimistic with an exclusion", version: "~> 3.0, != 3.2.0"},
		{name: "lower bound with a pessimistic", version: ">= 3.0, ~> 3.1"},
		{name: "two pessimistic precisions", version: "~> 3.1, ~> 3.2.0"},
		{name: "exact", version: "3.2.3"},
		{name: "exact with equals", version: "= 3.2.3"},
		{name: "exact with two parts", version: "3.2"},
		{name: "exclusion", version: "!= 3.2.0"},
		{name: "upper bound with two parts", version: "< 3.2"},
		{name: "inclusive upper bound", version: "<= 3.1"},
		{name: "exclusive lower bound", version: "> 3.1"},
		{name: "numeric build metadata out of order", version: ">= 3.0.0+10, >= 3.0.0+2"},
		{name: "numeric build metadata in order", version: ">= 3.0.0+2, >= 3.0.0+10"},
		{name: "numeric and text build metadata", version: ">= 3.0.0+1a, >= 3.0.0+2"},
		{name: "build metadata with one and two identifiers", version: ">= 3.0.0+a.1, >= 3.0.0+b"},
		{
			name:    "build metadata differing before the last identifier",
			version: ">= 3.0.0+b.1, >= 3.0.0+a.x.y",
		},
		{
			name:    "build metadata differing in the second identifier",
			version: ">= 3.0.0+build.10, >= 3.0.0+build.2",
		},
		{name: "build metadata sharing a version with none", version: ">= 3.0.0+build, >= 3.0.0"},
		{name: "build metadata with different operators", version: "> 3.0.0+2, != 3.0.0+1"},
		{
			name:    "prerelease with one and two identifiers",
			version: ">= 3.0.0-alpha.1, >= 3.0.0-beta",
		},
		{name: "numeric prerelease", version: ">= 3.0.0-10, >= 3.0.0-2"},
		{name: "numeric and text prerelease", version: ">= 3.0.0-alpha, >= 3.0.0-1"},
		{
			name:    "prerelease differing before the last identifier",
			version: ">= 3.0.0-beta.1, >= 3.0.0-alpha.x.y",
		},
		{
			name:    "prerelease differing in the second identifier",
			version: ">= 3.0.0-alpha.10, >= 3.0.0-alpha.2",
		},
		{name: "prerelease with build metadata", version: ">= 3.0.0-beta+1, >= 3.0.0-alpha+2"},
		{name: "dash with no prerelease after it", version: ">= 3.0-"},
		{name: "pessimistic with a dash and no prerelease after it", version: "~> 3.0-"},
		{name: "prerelease of a dash", version: ">= 3.0--"},
		{name: "prerelease starting with a dot", version: ">= 3.0-.a"},
		{
			name:    "dash with no prerelease sharing a version with a prerelease",
			version: ">= 3.0-, >= 3.0.0-beta",
		},
		{
			name:    "prerelease sharing a version with its release",
			version: ">= 3.0.0+1, >= 3.0.0-beta+2",
		},
	}

	// Every case shares one provider cache, and tofu re-installs from it, so
	// each provider version is downloaded once for the whole test instead of
	// once per command.
	providerCacheDir := helpers.TmpDirWOSymlinks(t)
	pluginCacheEnv := tf.EnvNameTFPluginCacheDir + "=" + providerCacheDir

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// The source has no registry host, so the wrapped binary resolves
			// its own default registry and the provider cache can intercept it.
			mainTf := `terraform {
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "` + tc.version + `"
    }
  }
}
`

			cachedPath := helpers.TmpDirWOSymlinks(t)
			require.NoError(
				t,
				os.WriteFile(filepath.Join(cachedPath, "main.tf"), []byte(mainTf), 0644),
			)
			require.NoError(t, os.WriteFile(filepath.Join(cachedPath, "terragrunt.hcl"), nil, 0644))

			directPath := helpers.TmpDirWOSymlinks(t)
			require.NoError(
				t,
				os.WriteFile(filepath.Join(directPath, "main.tf"), []byte(mainTf), 0644),
			)

			helpers.RunTerragrunt(
				t,
				fmt.Sprintf(
					"terragrunt init --provider-cache --provider-cache-dir %s --non-interactive --working-dir %s",
					providerCacheDir,
					cachedPath,
				),
			)
			runWrappedBinary(t, directPath, nil, "init", "-input=false")

			assert.Equal(
				t,
				extractConstraintsFromLockFile(t, directPath, "hashicorp/null"),
				extractConstraintsFromLockFile(t, cachedPath, "hashicorp/null"),
			)

			lockfilePath := filepath.Join(cachedPath, ".terraform.lock.hcl")
			cachedLockfile, err := os.ReadFile(lockfilePath)
			require.NoError(t, err)

			require.NoError(t, os.RemoveAll(filepath.Join(cachedPath, ".terraform")))
			runWrappedBinary(
				t,
				cachedPath,
				[]string{pluginCacheEnv},
				"init",
				"-input=false",
				"-lockfile=readonly",
			)
			runWrappedBinary(t, cachedPath, []string{pluginCacheEnv}, "init", "-input=false")

			reinitLockfile, err := os.ReadFile(lockfilePath)
			require.NoError(t, err)
			assert.Equal(t, string(cachedLockfile), string(reinitLockfile))
		})
	}
}

// runWrappedBinary runs tofu, or whichever binary Terragrunt wraps, directly
// in dir with env added to the environment, and fails the test when the
// command fails.
func runWrappedBinary(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), helpers.WrappedBinary(t.Context()), args...)
	cmd.Dir = dir

	cmd.Env = append(os.Environ(), env...)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
}

// Helper function to extract constraints value from lock file
func extractConstraintsFromLockFile(t *testing.T, appPath string, providerName string) string {
	t.Helper()

	lockfilePath := filepath.Join(appPath, ".terraform.lock.hcl")
	require.FileExists(t, lockfilePath, "Lock file should exist")

	// Read and parse the lock file
	lockfileContent, err := os.ReadFile(lockfilePath)
	require.NoError(t, err)

	lockfile, diags := hclwrite.ParseConfig(
		lockfileContent,
		lockfilePath,
		hcl.Pos{Line: 1, Column: 1},
	)
	require.False(t, diags.HasErrors(), "Lock file should be valid HCL")

	// Find the provider block (handle both short and full provider names)
	var providerBlock *hclwrite.Block
	if strings.Contains(providerName, "/") {
		// Full name like "cloudflare/cloudflare"
		providerBlock = lockfile.Body().
			FirstMatchingBlock("provider", []string{"registry.terraform.io/" + providerName})
		if providerBlock == nil {
			// Try OpenTofu registry as well
			providerBlock = lockfile.Body().
				FirstMatchingBlock("provider", []string{"registry.opentofu.org/" + providerName})
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
