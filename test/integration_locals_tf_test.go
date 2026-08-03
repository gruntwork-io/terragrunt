//go:build tf

package test_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFLocalsParsing(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLocalsCanonical)
	rootPath := filepath.Join(tmpEnvPath, testFixtureLocalsCanonical)
	helpers.CleanupTerraformFolder(t, rootPath)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -no-color -json --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "Hello world\n", outputs["data"].Value)
	assert.InEpsilon(t, 42.0, outputs["answer"].Value, 0.0000000001)
}

func TestTFLocalsInInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalsInInclude)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLocalsInInclude)
	childPath := filepath.Join(
		tmpEnvPath,
		testFixtureLocalsInInclude,
		testFixtureLocalsInIncludeChildRelPath,
	)
	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve -no-color --non-interactive --working-dir "+childPath,
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -no-color -json --non-interactive --working-dir "+childPath,
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(
		t,
		filepath.Join(tmpEnvPath, testFixtureLocalsInInclude),
		outputs["parent_terragrunt_dir"].Value,
	)
	assert.Equal(
		t,
		childPath,
		outputs["terragrunt_dir"].Value,
	)
	assert.Equal(
		t,
		"apply",
		outputs["terraform_command"].Value,
	)
	assert.Equal(
		t,
		"[\"apply\",\"-auto-approve\",\"-no-color\"]",
		outputs["terraform_cli_args"].Value,
	)
}

func TestTFTerragruntInitRunCmd(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalRunMultiple)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLocalRunMultiple)
	rootPath := filepath.Join(tmpEnvPath, testFixtureLocalRunMultiple)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt init --working-dir "+rootPath,
	)
	require.Error(t, err)

	// Check for cached values between locals and inputs sections
	assert.Equal(t, 1, strings.Count(stdout, "echo_potato"))
	assert.Equal(t, 1, strings.Count(stdout, "echo_carrot"))
	assert.Equal(t, 1, strings.Count(stdout, "echo_bar"))
	assert.Equal(t, 1, strings.Count(stdout, "echo_foo"))

	assert.Equal(t, 1, strings.Count(stdout, "echo_input_variable"))

	assert.Contains(t, stdout, "echo_uuid_input")
	assert.Contains(t, stdout, "echo_uuid_locals")
	assert.Contains(t, stdout, "echo_random_arg")
	assert.Contains(t, stdout, "echo_another_arg")
}
