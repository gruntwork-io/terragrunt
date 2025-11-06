package test_test

import (
	"os"
	"path/filepath"
	"strings"
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
// 1. When running without --all flag on directory which contains only terragrunt.stack.hcl
// 2. When running with --all but --no-stack-generate flag is set on directory which contains only terragrunt.stack.hcl
// 3. When running without --all flag on standard terragrunt directory
// 4. When running with --all but --no-stack-generate on directory without terragrunt.stack.hcl
func TestRunNoStacksGenerate(t *testing.T) {
	t.Parallel()

	// Define test cases for different scenarios where stack generation should be skipped
	testdata := []struct {
		name       string
		cmd        string
		subfolder  string
		shouldFail bool
	}{
		{
			name:       "NoAll",
			cmd:        "terragrunt run apply --non-interactive",
			subfolder:  "live",
			shouldFail: true,
		},
		{
			name:       "AllNoGenerate",
			cmd:        "terragrunt run apply --all --no-stack-generate --non-interactive",
			subfolder:  "live",
			shouldFail: false,
		},
		{
			name:       "Standard",
			cmd:        "terragrunt run apply --non-interactive",
			subfolder:  "units/chicken",
			shouldFail: false,
		},
		{
			name:       "AllNoStackToGenerate",
			cmd:        "terragrunt run apply --all --no-stack-generate --non-interactive",
			subfolder:  "units",
			shouldFail: false,
		},
	}

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)

	// Run each test case and verify stack generation is skipped
	for _, tt := range testdata {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Set up test environment
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
			path := util.JoinPath(tmpEnvPath, testFixtureStacksBasic, tt.subfolder)
			cmd := tt.cmd + " --working-dir " + path + " -- -auto-approve"

			// Execute terragrunt command and verify no output
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			if tt.shouldFail {
				require.Error(t, err)
				assert.Empty(t, stdout)
				// We should explicitly avoid asserting on stderr, because information
				// might be logged to stderr, even if the command succeeds.
				//
				// e.g. Usage of the provider cache server.
				//
				// assert.Empty(t, stderr)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, stderr)
			}

			// Verify that stack directory was not created
			genPath := util.JoinPath(path, ".terragrunt-stack")
			assert.NoDirExists(t, genPath)
		})
	}
}

func TestRunVersionFilesCacheKey(t *testing.T) {
	t.Parallel()

	testdata := []struct {
		name         string
		expect       string
		versionFiles []string
	}{
		{
			name:         "use default",
			expect:       "r01AJjVD7VSXCQk1ORuh_no_NRY",
			versionFiles: nil,
		},
		{
			name:   "custom files provided",
			expect: "XBE-VO9pOnQjPQDmLQCvSCdckSQ",
			versionFiles: []string{
				".terraform-version",
				".tool-versions",
			},
		},
	}

	helpers.CleanupTerraformFolder(t, testFixtureVersionFilesCacheKey)

	for _, tt := range testdata {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVersionFilesCacheKey, tt.versionFiles...)
			path := util.JoinPath(tmpEnvPath, testFixtureVersionFilesCacheKey)
			flags := []string{
				"-non-interactive",
				"--log-level debug",
				"--working-dir",
				path,
			}

			for _, file := range tt.versionFiles {
				flags = append(
					flags,
					"--version-manager-file-name",
					file,
				)
			}

			cmd := "terragrunt run apply " + strings.Join(flags, " ") + " -- -auto-approve"

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)
			assert.NotEmpty(t, stdout)
			assert.NotEmpty(t, stderr)
			assert.Contains(t, stderr, "using cache key for version files: "+tt.expect)
		})
	}
}
