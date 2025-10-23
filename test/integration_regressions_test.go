package test_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureRegressions            = "fixtures/regressions"
	testFixtureDependencyGenerate     = "fixtures/regressions/dependency-generate"
	testFixtureNestedStacksRunnerPool = "fixtures/regressions/nested-stacks-runner-pool"
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

// TestDependencyOutputInGenerateBlock tests that dependency outputs can be used in generate blocks.
// This is a regression test for issue #4962 where using dependency outputs in generate blocks
// started failing with "Unsuitable value: value must be known" error in v0.89.0+.
//
// The bug occurred because during `run --all`, the discovery phase was calling ParseConfigFile
// instead of PartialParseConfigFile, which caused generate blocks to be evaluated before
// dependency outputs were resolved. The fix ensures generate blocks are only evaluated when
// each unit runs individually with full dependency resolution.
//
// See: https://github.com/gruntwork-io/terragrunt/issues/4962
func TestDependencyOutputInGenerateBlock(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyGenerate)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDependencyGenerate)
	otherPath := util.JoinPath(rootPath, "other")
	testingPath := util.JoinPath(rootPath, "testing")

	helpers.CleanupTerraformFolder(t, rootPath)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply --non-interactive --working-dir "+otherPath+" -- -auto-approve",
	)

	_, runAllStderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all plan --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assert.NotContains(t, runAllStderr, "Unsuitable value: value must be known",
		"Should not fail with 'Unsuitable value' error when using dependency outputs in generate blocks")
	assert.NotContains(t, runAllStderr, "Unsuitable value type",
		"Should not fail with 'Unsuitable value type' error")

	// Verify the generate block was created successfully
	generatedFile := util.JoinPath(testingPath, ".terragrunt-cache")
	assert.DirExists(t, generatedFile, "Terragrunt cache should exist")
}

// TestDependencyOutputInGenerateBlockDirectRun tests that dependency outputs work when running directly
// This test verifies that even in the broken version, running directly (without --all) works
func TestDependencyOutputInGenerateBlockDirectRun(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyGenerate)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDependencyGenerate)
	otherPath := util.JoinPath(rootPath, "other")
	testingPath := util.JoinPath(rootPath, "testing")

	helpers.CleanupTerraformFolder(t, rootPath)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply --auto-approve --non-interactive --working-dir "+otherPath,
	)

	_, planStderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --working-dir "+testingPath,
	)
	require.NoError(t, err)

	assert.NotContains(t, planStderr, "Unsuitable value",
		"Direct run should never fail with 'Unsuitable value' error")
}

// TestDependencyOutputInInputsStillWorks verifies that dependency outputs can be used in inputs
func TestDependencyOutputInInputsStillWorks(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyGenerate)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDependencyGenerate)
	otherPath := util.JoinPath(rootPath, "other")

	// Apply the "other" module
	helpers.CleanupTerraformFolder(t, rootPath)

	helpers.RunTerragrunt(t,
		"terragrunt apply --auto-approve --non-interactive --working-dir "+otherPath,
	)

	runAllStdout, runAllStderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all apply --non-interactive --working-dir "+rootPath+" -- --auto-approve",
	)
	require.NoError(t, err)

	assert.True(t, strings.Contains(runAllStdout, "test-token-12345") ||
		strings.Contains(runAllStderr, "test-token-12345"),
		"Token should be passed via inputs")
}

// Regression test for https://github.com/gruntwork-io/terragrunt/issues/4977
// Ensures runner pool includes all units generated from nested stacks.
func TestRunnerPoolIncludesAllNestedStackUnits(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNestedStacksRunnerPool)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStacksRunnerPool)
	gitPath := util.JoinPath(tmpEnvPath, testFixtureNestedStacksRunnerPool)
	helpers.CreateGitRepo(t, gitPath)
	rootPath := util.JoinPath(gitPath, "live")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all plan --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	// Expect all units from top-level and nested stacks to be present in the runner order
	expectedUnits := []string{
		"- Unit ./.terragrunt-stack/id",
		"- Unit ./.terragrunt-stack/ecr-cache",
		"- Unit ./.terragrunt-stack/network/.terragrunt-stack/vpc",
		"- Unit ./.terragrunt-stack/network/.terragrunt-stack/vpc-nat",
		"- Unit ./.terragrunt-stack/network/.terragrunt-stack/vpc-endpoints",
		"- Unit ./.terragrunt-stack/network/.terragrunt-stack/tailscale-router",
		"- Unit ./.terragrunt-stack/k8s/.terragrunt-stack/eks-cluster",
		"- Unit ./.terragrunt-stack/k8s/.terragrunt-stack/eks-baseline",
		"- Unit ./.terragrunt-stack/k8s/.terragrunt-stack/grafana-baseline",
		"- Unit ./.terragrunt-stack/k8s/.terragrunt-stack/rancher-bootstrap",
		"- Unit ./.terragrunt-stack/k8s/.terragrunt-stack/rancher-baseline",
	}

	// Extract the runner pool order from stderr for a clearer failure message
	var actualOrder []string

	inOrder := false

	for _, line := range strings.Split(stderr, "\n") {
		if strings.Contains(line, "The runner-pool runner at ") {
			inOrder = true
			continue
		}

		if inOrder {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- Unit ") {
				actualOrder = append(actualOrder, trimmed)
				continue
			}

			if trimmed == "" {
				// End of the runner pool listing block
				inOrder = false
			}
		}
	}

	// Build a quick lookup set for actual entries
	actualSet := make(map[string]struct{}, len(actualOrder))
	for _, u := range actualOrder {
		actualSet[u] = struct{}{}
	}

	// Compute missing expectations
	var missing []string

	for _, expected := range expectedUnits {
		if _, ok := actualSet[expected]; !ok {
			missing = append(missing, expected)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("Missing units in runner pool:%s\n\nActual runner pool order:%s\n\nFull stderr:%s",
			"\n- "+strings.Join(missing, "\n- "),
			"\n"+strings.Join(actualOrder, "\n"),
			"\n"+stderr,
		)
	}

	// Keep the per-item assertion for extra signal in the test logs
	for _, expected := range expectedUnits {
		assert.Contains(t, stderr, expected)
	}
}
