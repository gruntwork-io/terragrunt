package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureLocalsErrorUndefinedLocal         = "fixtures/locals-errors/undefined-local"
	testFixtureLocalsErrorUndefinedLocalButInput = "fixtures/locals-errors/undefined-local-but-input"
	testFixtureLocalsCanonical                   = "fixtures/locals/canonical"
	testFixtureLocalsInInclude                   = "fixtures/locals/local-in-include"
	testFixtureLocalRunOnce                      = "fixtures/locals/run-once"
	testFixtureLocalRunMultiple                  = "fixtures/locals/run-multiple"
	testFixtureLocalsInIncludeChildRelPath       = "qa/my-app"
	testFixtureBrokenLocals                      = "fixtures/broken-locals"
)

func TestUndefinedLocalsReferenceBreaks(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalsErrorUndefinedLocal)
	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureLocalsErrorUndefinedLocal, os.Stdout, os.Stderr)
	require.Error(t, err)
}

func TestUndefinedLocalsReferenceToInputsBreaks(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalsErrorUndefinedLocalButInput)
	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureLocalsErrorUndefinedLocalButInput, os.Stdout, os.Stderr)
	require.Error(t, err)
}

func TestLocalsParsing(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalsCanonical)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureLocalsCanonical)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --non-interactive --working-dir "+testFixtureLocalsCanonical)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "Hello world\n", outputs["data"].Value)
	assert.InEpsilon(t, 42.0, outputs["answer"].Value, 0.0000000001)
}

func TestLocalsInInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalsInInclude)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLocalsInInclude)
	childPath := filepath.Join(tmpEnvPath, testFixtureLocalsInInclude, testFixtureLocalsInIncludeChildRelPath)
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve -no-color --non-interactive --working-dir "+childPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --non-interactive --working-dir "+childPath)
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

func TestLogFailedLocalsEvaluation(t *testing.T) {
	t.Parallel()

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --working-dir %s --log-level trace", testFixtureBrokenLocals), &stdout, &stderr)
	require.Error(t, err)

	output := stderr.String()
	assert.Contains(t, output, "Encountered error while evaluating locals in file "+filepath.FromSlash("./terragrunt.hcl"))
}

func TestTerragruntInitRunCmd(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalRunMultiple)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLocalRunMultiple)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLocalRunMultiple)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt init --working-dir "+rootPath)
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

func TestTerragruntLocalRunOnce(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLocalRunOnce)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt init --working-dir "+testFixtureLocalRunOnce, &stdout, &stderr)
	require.Error(t, err)

	errout := stdout.String()

	assert.Equal(t, 1, strings.Count(errout, "foo"))
}
