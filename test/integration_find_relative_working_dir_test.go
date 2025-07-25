package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindCommandWithRelativeWorkingDir(t *testing.T) {
	t.Parallel()

	// This test validates the fix for the reported issue where:
	// "terragrunt find --working-dir ./deploy-2" fails with "Rel: can't make <X> relative to <Y>" errors

	// Create a temporary directory structure that matches the bug report
	tmpDir, err := os.MkdirTemp("", "terragrunt-find-relative-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Save current directory to restore later
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	// Change to tmpDir to make relative paths work
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Create the directory structure as described in the bug report
	deploy1Dir := "deploy-1"
	deploy2Dir := "deploy-2"
	require.NoError(t, os.MkdirAll(deploy1Dir, 0o755))
	require.NoError(t, os.MkdirAll(deploy2Dir, 0o755))

	// Create foo.hcl in the root (parent directory)
	fooHclContent := `# Parent configuration file`
	require.NoError(t, os.WriteFile("foo.hcl", []byte(fooHclContent), 0o644))

	// Create deploy-1/terragrunt.hcl
	deploy1TerragruntHcl := `terraform {
  source = "./"
}

include "foo" {
  path = "./foo.hcl"
}`
	require.NoError(t, os.WriteFile(filepath.Join(deploy1Dir, "terragrunt.hcl"), []byte(deploy1TerragruntHcl), 0o644))

	// Create deploy-2/terragrunt.hcl with dependency (this is the key scenario from the bug report)
	deploy2TerragruntHcl := `terraform {
  source = "./"
}

include "foo" {
  path = "./foo.hcl"
}

dependency "dep1" {
  config_path = find_in_parent_folders("deploy-1")
}`
	require.NoError(t, os.WriteFile(filepath.Join(deploy2Dir, "terragrunt.hcl"), []byte(deploy2TerragruntHcl), 0o644))

	// Create simple terraform files in each directory
	simpleTfContent := `resource "null_resource" "example" {}`
	require.NoError(t, os.WriteFile(filepath.Join(deploy1Dir, "main.tf"), []byte(simpleTfContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(deploy2Dir, "main.tf"), []byte(simpleTfContent), 0o644))

	// Test different relative working directory formats that should all work now
	testCases := []struct {
		name       string
		workingDir string
	}{
		{"with dot slash", "./deploy-2"},
		{"without dot slash", "deploy-2"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the exact command from the bug report (this was failing before our fix)
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, 
				"terragrunt find --working-dir "+tc.workingDir+" --external --include --dependencies --json")
			
			// The main fix validation: command should succeed (no more filepath.Rel errors)
			require.NoError(t, err, "Find command should succeed with relative working directory: %s", tc.workingDir)
			assert.Empty(t, stderr, "Should not have any error output")

			// Parse and validate the JSON output
			var findResults find.FoundConfigs
			err = json.Unmarshal([]byte(stdout), &findResults)
			require.NoError(t, err, "Should be able to parse JSON output")

			// Should find 2 modules (deploy-1 as external dependency and deploy-2 as main)
			require.Len(t, findResults, 2, "Should find 2 modules (deploy-1 and deploy-2)")

			// Verify the structure matches expectations
			modulesByPath := make(map[string]*find.FoundConfig)
			for _, module := range findResults {
				modulesByPath[module.Path] = module
			}

			// deploy-1 should be included as an external dependency
			deploy1Module, hasDeploy1 := modulesByPath["../deploy-1"]
			require.True(t, hasDeploy1, "Should find deploy-1 module")
			assert.Equal(t, "unit", string(deploy1Module.Type))

			// deploy-2 should be the main module
			deploy2Module, hasDeploy2 := modulesByPath["."]
			require.True(t, hasDeploy2, "Should find deploy-2 module")
			assert.Equal(t, "unit", string(deploy2Module.Type))
			assert.Contains(t, deploy2Module.Dependencies, "../deploy-1", "Should have dependency on deploy-1")
		})
	}
} 