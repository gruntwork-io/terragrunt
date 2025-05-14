package test_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunStacksBasic verifies that Terragrunt can:
// 1. Generate a stack from a terragrunt.stack.hcl configuration
// 2. Apply the stack successfully
// 3. Create the expected output files with correct contents
func TestRunStacksBasic(t *testing.T) {
	t.Parallel()

	// Setup: Clean up and create a temporary copy of the test fixture
	fixture := "fixtures/run-stack"
	helpers.CleanupTerraformFolder(t, fixture)
	tmpEnvPath := helpers.CopyEnvironment(t, fixture)
	rootPath := util.JoinPath(tmpEnvPath, fixture, "")
	defer helpers.CleanupTerraformFolder(t, rootPath)

	// Run terragrunt apply on the stack
	cmd := "terragrunt run apply --all --non-interactive --working-dir " + rootPath
	helpers.RunTerragrunt(t, cmd)

	// Validate the generated stack structure
	stackPath := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, stackPath)

	// Verify that all expected output files were created
	var txtFiles []string
	err := filepath.Walk(stackPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "test.txt" {
			txtFiles = append(txtFiles, filePath)
		}
		return nil
	})

	require.NoError(t, err, "Failed to walk stack directory")
	assert.Len(t, txtFiles, 4, "Expected 4 test.txt files in the stack directory")
}
