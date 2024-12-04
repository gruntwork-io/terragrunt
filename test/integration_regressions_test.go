package test_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureRegressions = "fixtures/regressions"
)

func TestNoAutoInit(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRegressions)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRegressions)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRegressions, "skip-init")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt apply --terragrunt-no-auto-init --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "no force apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "no force apply stderr")
	require.Error(t, err)
	assert.Contains(t, stderr.String(), "This module is not yet installed.")
}

// Test case for yamldecode bug: https://github.com/gruntwork-io/terragrunt/issues/834
func TestYamlDecodeRegressions(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRegressions)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRegressions)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRegressions, "yamldecode")

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// Check the output of yamldecode and make sure it doesn't parse the string incorrectly
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "003", outputs["test1"].Value)
	assert.Equal(t, "1.00", outputs["test2"].Value)
	assert.Equal(t, "0ba", outputs["test3"].Value)
}

func TestMockOutputsMergeWithState(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRegressions)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRegressions)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRegressions, "mocks-merge-with-state")

	modulePath := util.JoinPath(rootPath, "module")
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt apply --terragrunt-log-level trace --terragrunt-non-interactive -auto-approve --terragrunt-working-dir "+modulePath, &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "module-executed")
	require.NoError(t, err)

	deepMapPath := util.JoinPath(rootPath, "deep-map")
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	err = helpers.RunTerragruntCommand(t, "terragrunt apply --terragrunt-log-level trace --terragrunt-non-interactive -auto-approve --terragrunt-working-dir "+deepMapPath, &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "deep-map-executed")
	require.NoError(t, err)

	shallowPath := util.JoinPath(rootPath, "shallow")
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	err = helpers.RunTerragruntCommand(t, "terragrunt apply --terragrunt-log-level trace --terragrunt-non-interactive -auto-approve --terragrunt-working-dir "+shallowPath, &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "shallow-map-executed")
	require.NoError(t, err)
}
