package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureLocalsErrorUndefinedLocal         = "fixtures/locals-errors/undefined-local"
	testFixtureLocalsErrorUndefinedLocalButInput = "fixtures/locals-errors/undefined-local-but-input"
	testFixtureLocalsCanonical                   = "fixtures/locals/canonical"
	testFixtureLocalsInInclude                   = "fixtures/locals/local-in-include"
	TestFixtureLocalRunOnce                      = "fixtures/locals/run-once"
	testFixtureLocalRunMultiple                  = "fixtures/locals/run-multiple"
	testFixtureLocalsInIncludeChildRelPath       = "qa/my-app"
	testFixtureBrokenLocals                      = "fixtures/broken-locals"
)

func TestUndefinedLocalsReferenceBreaks(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureLocalsErrorUndefinedLocal)
	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureLocalsErrorUndefinedLocal, os.Stdout, os.Stderr)
	require.Error(t, err)
}

func TestUndefinedLocalsReferenceToInputsBreaks(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureLocalsErrorUndefinedLocalButInput)
	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureLocalsErrorUndefinedLocalButInput, os.Stdout, os.Stderr)
	require.Error(t, err)
}

func TestLocalsParsing(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureLocalsCanonical)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureLocalsCanonical)

	stdout, _, err := runTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureLocalsCanonical)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "Hello world\n", outputs["data"].Value)
	assert.InEpsilon(t, 42.0, outputs["answer"].Value, 0.0000000001)
}

func TestLocalsInInclude(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureLocalsInInclude)
	tmpEnvPath := copyEnvironment(t, testFixtureLocalsInInclude)
	childPath := filepath.Join(tmpEnvPath, testFixtureLocalsInInclude, testFixtureLocalsInIncludeChildRelPath)
	runTerragrunt(t, "terragrunt apply -auto-approve -no-color --terragrunt-non-interactive --terragrunt-working-dir "+childPath)

	stdout, _, err := runTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+childPath)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
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

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-log-level debug", testFixtureBrokenLocals), &stdout, &stderr)
	require.Error(t, err)

	output := stderr.String()
	assert.Contains(t, output, "Encountered error while evaluating locals in file ./terragrunt.hcl")
}

func TestTerragruntInitRunCmd(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureLocalRunMultiple)
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt init --terragrunt-working-dir "+testFixtureLocalRunMultiple, &stdout, &stderr)
	require.Error(t, err)

	errout := stdout.String()

	// Check for cached values between locals and inputs sections
	assert.Equal(t, 1, strings.Count(errout, "potato"))
	assert.Equal(t, 1, strings.Count(errout, "carrot"))
	assert.Equal(t, 1, strings.Count(errout, "bar"))
	assert.Equal(t, 1, strings.Count(errout, "foo"))

	assert.Equal(t, 1, strings.Count(errout, "input_variable"))

	// Commands executed multiple times because of different arguments
	assert.Equal(t, 4, strings.Count(errout, "uuid"))
	assert.Equal(t, 6, strings.Count(errout, "random_arg"))
	assert.Equal(t, 4, strings.Count(errout, "another_arg"))
}

func TestTerragruntLocalRunOnce(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TestFixtureLocalRunOnce)
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt init --terragrunt-working-dir "+TestFixtureLocalRunOnce, &stdout, &stderr)
	require.Error(t, err)

	errout := stdout.String()

	assert.Equal(t, 1, strings.Count(errout, "foo"))
}
