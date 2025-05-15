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

// TestRunStacksGenerate verifies that stack generation works correctly when running terragrunt with --all flag.
// It ensures that:
// 1. The stack directory is created
// 2. The stack is properly applied
// 3. The expected number of test.txt files are generated
func TestRunStacksGenerate(t *testing.T) {
	t.Parallel()

	// Set up test environment
	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksBasic, "live")

	// Run terragrunt with --all flag to trigger stack generation
	helpers.RunTerragrunt(t, "terragrunt run apply --all --non-interactive --working-dir "+rootPath)

	// Verify stack directory exists and validate its contents
	path := util.JoinPath(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)

	// Collect all test.txt files in the stack directory to verify correct generation
	var txtFiles []string
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "test.txt" {
			txtFiles = append(txtFiles, filePath)
		}
		return nil
	})

	require.NoError(t, err)
	// Verify that exactly 4 test.txt files were generated
	assert.Len(t, txtFiles, 4)
}

// TestRunNoStacksGenerate verifies that stack generation is skipped in appropriate scenarios:
// 1. When running without --all flag
// 2. When running with --all but --no-stack-generate flag is set
func TestRunNoStacksGenerate(t *testing.T) {
	t.Parallel()

	// Set up test environment
	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksBasic, "live")

	// Define test cases for different scenarios where stack generation should be skipped
	testdata := []struct {
		name string
		cmd  string
	}{
		{
			name: "NoAll",
			cmd:  "terragrunt run apply --non-interactive --working-dir " + rootPath,
		},
		{
			name: "AllNoGenerate",
			cmd:  "terragrunt run apply --all --no-stack-generate --non-interactive --working-dir " + rootPath,
		},
	}

	// Run each test case and verify stack generation is skipped
	for _, tt := range testdata {
		t.Run(tt.name, func(t *testing.T) {
			// Execute terragrunt command and verify no output
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, tt.cmd)
			require.NoError(t, err)
			assert.Empty(t, stdout)
			assert.Empty(t, stderr)

			// Verify that stack directory was not created
			path := util.JoinPath(rootPath, ".terragrunt-stack")
			assert.NoDirExists(t, path)
		})
	}
}
