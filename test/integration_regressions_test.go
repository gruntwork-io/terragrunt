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
	err := helpers.RunTerragruntCommand(t, "terragrunt apply --no-auto-init --log-level trace --non-interactive --working-dir "+rootPath, &stdout, &stderr)
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

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	// Check the output of yamldecode and make sure it doesn't parse the string incorrectly
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+rootPath, &stdout, &stderr),
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
	err := helpers.RunTerragruntCommand(t, "terragrunt apply --log-level trace --non-interactive -auto-approve --working-dir "+modulePath, &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "module-executed")
	require.NoError(t, err)

	deepMapPath := util.JoinPath(rootPath, "deep-map")
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	err = helpers.RunTerragruntCommand(t, "terragrunt apply --log-level trace --non-interactive -auto-approve --working-dir "+deepMapPath, &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "deep-map-executed")
	require.NoError(t, err)

	shallowPath := util.JoinPath(rootPath, "shallow")
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	err = helpers.RunTerragruntCommand(t, "terragrunt apply --log-level trace --non-interactive -auto-approve --working-dir "+shallowPath, &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "shallow-map-executed")
	require.NoError(t, err)
}

func TestIncludeError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRegressions)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRegressions)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRegressions, "include-error", "project", "app")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --non-interactive --working-dir "+rootPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "include blocks without label")
}

// Test case for shallow merge copy filters bug: https://github.com/gruntwork-io/terragrunt/issues/4757
// When using shallow merge (default), child's include_in_copy/exclude_from_copy values were being dropped
// if parent had a terraform block but no copy filters defined.
func TestShallowMergeCopyFilters(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRegressions)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRegressions)
	fixturePath := util.JoinPath(tmpEnvPath, testFixtureRegressions, "shallow-merge-copy-filters-4757")

	t.Run("parent_nil_child_set", func(t *testing.T) {
		t.Parallel()
		rootPath := util.JoinPath(fixturePath, "app")

		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}
		err := helpers.RunTerragruntCommand(t, "terragrunt render --json --non-interactive --working-dir "+rootPath, &stdout, &stderr)
		require.NoError(t, err)

		var rendered map[string]any
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &rendered))

		terraformBlock, hasTerraform := rendered["terraform"]
		require.True(t, hasTerraform, "terraform block should be present")

		tfMap := terraformBlock.(map[string]any)

		// Verify source from parent is preserved
		source, hasSource := tfMap["source"]
		assert.True(t, hasSource, "terraform.source should be present")
		assert.Equal(t, "./modules/example", source)

		// Verify child's exclude_from_copy is preserved (this was the bug)
		excludeFromCopy, hasExclude := tfMap["exclude_from_copy"]
		assert.True(t, hasExclude, "terraform.exclude_from_copy should be present")
		assert.Equal(t, []any{"**/_*"}, excludeFromCopy)

		// Verify child's include_in_copy is preserved
		includeInCopy, hasInclude := tfMap["include_in_copy"]
		assert.True(t, hasInclude, "terraform.include_in_copy should be present")
		assert.Equal(t, []any{"special-file.txt"}, includeInCopy)
	})

	t.Run("both_set_child_wins", func(t *testing.T) {
		t.Parallel()
		// Scenario: both parent and child define copy filters
		// Expected: child values should completely override parent (no concatenation in shallow merge)
		rootPath := util.JoinPath(fixturePath, "both-set")

		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}
		err := helpers.RunTerragruntCommand(t, "terragrunt render --json --non-interactive --working-dir "+rootPath, &stdout, &stderr)
		require.NoError(t, err)

		var rendered map[string]any
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &rendered))

		terraformBlock, hasTerraform := rendered["terraform"]
		require.True(t, hasTerraform, "terraform block should be present")

		tfMap := terraformBlock.(map[string]any)

		// Verify source from parent is preserved
		source, hasSource := tfMap["source"]
		assert.True(t, hasSource, "terraform.source should be present")
		assert.Equal(t, "./modules/example", source)

		// In shallow merge, child values should completely override parent (not concatenate)
		excludeFromCopy, hasExclude := tfMap["exclude_from_copy"]
		assert.True(t, hasExclude, "terraform.exclude_from_copy should be present")
		assert.Equal(t, []any{"child-exclude/**"}, excludeFromCopy, "child should override parent, not concatenate")

		includeInCopy, hasInclude := tfMap["include_in_copy"]
		assert.True(t, hasInclude, "terraform.include_in_copy should be present")
		assert.Equal(t, []any{"child-include.txt"}, includeInCopy, "child should override parent, not concatenate")
	})
}
