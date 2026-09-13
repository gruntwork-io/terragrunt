//go:build exec

// These tests hand Terragrunt a shell script as the OpenTofu binary, so each
// one spawns a real process and is gated behind the exec tag.

package test_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureErrorPrint  = "fixtures/error-print"
	testFixtureTfPathBasic = "fixtures/tf-path/basic"
)

func TestExecErrorMessageIncludeInOutput(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since bash script execution is not supported")
	}

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureErrorPrint)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureErrorPrint)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply  --non-interactive --working-dir "+testPath+" --tf-path "+testPath+"/custom-tf-script.sh --log-level trace",
	)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "Custom error from script")
}

//nolint:paralleltest // it unsets TG_TF_PATH for the whole process
func TestExecTfPath(t *testing.T) {
	// This test can't be parallelized because it explicitly unsets the TG_TF_PATH environment variable.
	// t.Parallel()
	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since bash script execution is not supported")
	}

	// Test that the terragrunt run version command correctly identifies and uses
	// the terraform_binary path configuration if present
	helpers.CleanupTerraformFolder(t, testFixtureTfPathBasic)
	rootPath := helpers.CopyEnvironment(t, testFixtureTfPathBasic)
	workingDir := filepath.Join(rootPath, testFixtureTfPathBasic)
	workingDir, err := filepath.EvalSymlinks(workingDir)
	require.NoError(t, err)

	// If TG_TF_PATH is not set, we'll use the default tofu binary,
	// we'll explicitly set the value so that the test can pass.
	if tfPath := os.Getenv("TG_TF_PATH"); tfPath != "" {
		// Unset after using t.Setenv so that it'll be reset after the test.
		t.Setenv("TG_TF_PATH", "")
		os.Unsetenv("TG_TF_PATH")
	}

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run version --working-dir "+workingDir,
	)
	require.NoError(t, err)

	assert.Contains(t, stderr, "TF script used!")
}

func TestExecTfPathOverridesConfig(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since bash script execution is not supported")
	}
	// Test that the terragrunt run version command correctly identifies and uses
	// the terraform_binary path configuration if present
	helpers.CleanupTerraformFolder(t, testFixtureTfPathBasic)
	rootPath := helpers.CopyEnvironment(t, testFixtureTfPathBasic)
	workingDir := filepath.Join(rootPath, testFixtureTfPathBasic)
	workingDir, err := filepath.EvalSymlinks(workingDir)
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run version --tf-path ./other-tf.sh --working-dir "+workingDir,
	)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Other TF script used!")
}
