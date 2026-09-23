//go:build exec

// These tests hand Terragrunt a shell script as the OpenTofu binary, so each
// one spawns a real process and is gated behind the exec tag.

package test_test

import (
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

func TestExecTfPath(t *testing.T) {
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

	// The run has to fall through to the terraform_binary the config names, so it
	// starts without the TG_TF_PATH the suite may be running under.
	v := helpers.RunVenv(t)
	delete(v.Env, "TG_TF_PATH")

	_, stderr, err := helpers.RunTerragruntCommandWithOutputWithVenv(
		t,
		v,
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
