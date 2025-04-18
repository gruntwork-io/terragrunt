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
	includeDeepFixturePath      = "fixtures/include-deep/"
	includeDeepFixtureChildPath = "child"
	includeFixturePath          = "fixtures/include/"
	includeShallowFixturePath   = "stage/my-app"
	includeNoMergeFixturePath   = "qa/my-app"
	includeExposeFixturePath    = "fixtures/include-expose/"
	includeChildFixturePath     = "child"
	includeMultipleFixturePath  = "fixtures/include-multiple/"
	includeRunAllFixturePath    = "fixtures/include-runall/"
)

func TestTerragruntWorksWithIncludeLocals(t *testing.T) {
	t.Parallel()

	files, err := os.ReadDir(includeExposeFixturePath)
	require.NoError(t, err)

	testCases := []string{}
	for _, finfo := range files {
		if finfo.IsDir() {
			testCases = append(testCases, finfo.Name())
		}
	}

	for _, tc := range testCases {
		t.Run(filepath.Base(tc), func(t *testing.T) {
			t.Parallel()

			childPath := filepath.Join(includeExposeFixturePath, tc, includeChildFixturePath)
			helpers.CleanupTerraformFolder(t, childPath)
			helpers.RunTerragrunt(t, "terragrunt run-all apply -auto-approve --terragrunt-include-external-dependencies --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+childPath)

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}
			err := helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+childPath, &stdout, &stderr)
			require.NoError(t, err)

			outputs := map[string]helpers.TerraformOutput{}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
			assert.Equal(t, "us-west-1-test", outputs["region"].Value.(string))
		})
	}
}

func TestTerragruntRunAllModulesThatIncludeRestrictsSet(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, includeRunAllFixturePath)
	modulePath := util.JoinPath(rootPath, includeRunAllFixturePath)
	helpers.CleanupTerraformFolder(t, modulePath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		fmt.Sprintf(
			"terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-forward-tf-stdout --terragrunt-working-dir %s --terragrunt-modules-that-include alpha.hcl",
			modulePath,
		),
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "stderr")

	planOutput := stdout.String()
	assert.Contains(t, planOutput, "alpha")
	assert.NotContains(t, planOutput, "beta")
	assert.NotContains(t, planOutput, "charlie")
}

func TestTerragruntRunAllModulesWithPrefix(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, includeRunAllFixturePath)
	modulePath := util.JoinPath(rootPath, includeRunAllFixturePath)
	helpers.CleanupTerraformFolder(t, modulePath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt run-all plan --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+modulePath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "stderr")

	planOutput := stdout.String()
	assert.Contains(t, planOutput, "alpha")
	assert.Contains(t, planOutput, "beta")
	assert.Contains(t, planOutput, "charlie")

	stdoutLines := strings.Split(stderr.String(), "\n")
	for _, line := range stdoutLines {
		if strings.Contains(line, "alpha") {
			assert.Contains(t, line, "prefix=a")
		}
		if strings.Contains(line, "beta") {
			assert.Contains(t, line, "prefix=b")
		}
		if strings.Contains(line, "charlie") {
			assert.Contains(t, line, "prefix=c")
		}
	}
}

func TestTerragruntWorksWithIncludeDeepMerge(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(includeDeepFixturePath, "child")
	helpers.CleanupTerraformFolder(t, childPath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+childPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+childPath, &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "mock", outputs["attribute"].Value.(string))
	assert.Equal(t, "new val", outputs["new_attribute"].Value.(string))
	assert.Equal(t, "old val", outputs["old_attribute"].Value.(string))
	assert.Equal(t, []any{"hello", "mock"}, outputs["list_attr"].Value.([]any))
	assert.Equal(t, map[string]any{"foo": "bar", "bar": "baz", "test": "new val"}, outputs["map_attr"].Value.(map[string]any))

	assert.Equal(
		t,
		map[string]any{
			"attribute":     "mock",
			"new_attribute": "new val",
			"old_attribute": "old val",
			"list_attr":     []any{"hello", "mock"},
			"map_attr": map[string]any{
				"foo": "bar",
				"bar": "baz",
			},
		},
		outputs["dep_out"].Value.(map[string]any),
	)
}

func TestTerragruntWorksWithMultipleInclude(t *testing.T) {
	t.Parallel()

	files, err := os.ReadDir(includeMultipleFixturePath)
	require.NoError(t, err)

	testCases := []string{}
	for _, finfo := range files {
		if finfo.IsDir() && filepath.Base(finfo.Name()) != "modules" {
			testCases = append(testCases, finfo.Name())
		}
	}

	for _, tc := range testCases {
		t.Run(filepath.Base(tc), func(t *testing.T) {
			t.Parallel()

			childPath := filepath.Join(includeMultipleFixturePath, tc, includeDeepFixtureChildPath)
			helpers.CleanupTerraformFolder(t, childPath)
			helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+childPath)

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}
			err := helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+childPath, &stdout, &stderr)
			require.NoError(t, err)

			outputs := map[string]helpers.TerraformOutput{}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
			validateMultipleIncludeTestOutput(t, outputs)
		})
	}
}

func validateMultipleIncludeTestOutput(t *testing.T, outputs map[string]helpers.TerraformOutput) {
	t.Helper()

	assert.Equal(t, "mock", outputs["attribute"].Value.(string))
	assert.Equal(t, "new val", outputs["new_attribute"].Value.(string))
	assert.Equal(t, "old val", outputs["old_attribute"].Value.(string))
	assert.Equal(t, []any{"hello", "mock", "foo"}, outputs["list_attr"].Value.([]any))
	assert.Equal(t, map[string]any{"foo": "bar", "bar": "baz", "test": "new val"}, outputs["map_attr"].Value.(map[string]any))

	assert.Equal(
		t,
		map[string]any{
			"attribute":     "mock",
			"new_attribute": "new val",
			"old_attribute": "old val",
			"list_attr":     []any{"hello", "mock", "foo"},
			"map_attr": map[string]any{
				"foo": "bar",
				"bar": "baz",
			},
		},
		outputs["dep_out"].Value.(map[string]any),
	)
}

func validateIncludeRemoteStateReflection(t *testing.T, s3BucketName string, keyPath string, configPath string, workingDir string) {
	t.Helper()

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-config %s --terragrunt-working-dir %s", configPath, workingDir), &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	remoteStateOut := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(outputs["reflect"].Value.(string)), &remoteStateOut))
	assert.Equal(
		t,
		map[string]any{
			"backend":                         "s3",
			"disable_init":                    false,
			"disable_dependency_optimization": false,
			"generate":                        nil,
			"config": map[string]any{
				"encrypt": true,
				"bucket":  s3BucketName,
				"key":     keyPath + "/terraform.tfstate",
				"region":  "us-west-2",
			},
			"encryption": nil,
		},
		remoteStateOut,
	)
}
