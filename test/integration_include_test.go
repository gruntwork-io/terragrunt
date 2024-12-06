package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
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

	tc := []string{}
	for _, finfo := range files {
		if finfo.IsDir() {
			tc = append(tc, finfo.Name())
		}
	}

	for _, tt := range tc {
		// Capture range variable to avoid it changing across parallel test runs
		tt := tt

		t.Run(filepath.Base(tt), func(t *testing.T) {
			t.Parallel()

			childPath := filepath.Join(includeExposeFixturePath, tt, includeChildFixturePath)
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

func TestTerragruntWorksWithIncludeShallowMerge(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(includeFixturePath, includeShallowFixturePath)
	helpers.CleanupTerraformFolder(t, childPath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfigWithParentAndChild(t, includeFixturePath, includeShallowFixturePath, s3BucketName, "root.hcl", config.DefaultTerragruntConfigPath)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))
	validateIncludeRemoteStateReflection(t, s3BucketName, includeShallowFixturePath, tmpTerragruntConfigPath, childPath)
}

func TestTerragruntWorksWithIncludeNoMerge(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(includeFixturePath, includeNoMergeFixturePath)
	helpers.CleanupTerraformFolder(t, childPath)

	// We deliberately pick an s3 bucket name that is invalid, as we don't expect to create this s3 bucket.
	s3BucketName := "__INVALID_NAME__"

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfigWithParentAndChild(t, includeFixturePath, includeNoMergeFixturePath, s3BucketName, "root.hcl", config.DefaultTerragruntConfigPath)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))
	validateIncludeRemoteStateReflection(t, s3BucketName, includeNoMergeFixturePath, tmpTerragruntConfigPath, childPath)
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
	assert.Equal(t, []interface{}{"hello", "mock"}, outputs["list_attr"].Value.([]interface{}))
	assert.Equal(t, map[string]interface{}{"foo": "bar", "bar": "baz", "test": "new val"}, outputs["map_attr"].Value.(map[string]interface{}))

	assert.Equal(
		t,
		map[string]interface{}{
			"attribute":     "mock",
			"new_attribute": "new val",
			"old_attribute": "old val",
			"list_attr":     []interface{}{"hello", "mock"},
			"map_attr": map[string]interface{}{
				"foo": "bar",
				"bar": "baz",
			},
		},
		outputs["dep_out"].Value.(map[string]interface{}),
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

	for _, testCase := range testCases {
		// Capture range variable to avoid it changing across parallel test runs
		testCase := testCase

		t.Run(filepath.Base(testCase), func(t *testing.T) {
			t.Parallel()

			childPath := filepath.Join(includeMultipleFixturePath, testCase, includeDeepFixtureChildPath)
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
	assert.Equal(t, []interface{}{"hello", "mock", "foo"}, outputs["list_attr"].Value.([]interface{}))
	assert.Equal(t, map[string]interface{}{"foo": "bar", "bar": "baz", "test": "new val"}, outputs["map_attr"].Value.(map[string]interface{}))

	assert.Equal(
		t,
		map[string]interface{}{
			"attribute":     "mock",
			"new_attribute": "new val",
			"old_attribute": "old val",
			"list_attr":     []interface{}{"hello", "mock", "foo"},
			"map_attr": map[string]interface{}{
				"foo": "bar",
				"bar": "baz",
			},
		},
		outputs["dep_out"].Value.(map[string]interface{}),
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
	remoteStateOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["reflect"].Value.(string)), &remoteStateOut))
	assert.Equal(
		t,
		map[string]interface{}{
			"backend":                         "s3",
			"disable_init":                    false,
			"disable_dependency_optimization": false,
			"generate":                        nil,
			"config": map[string]interface{}{
				"encrypt": true,
				"bucket":  s3BucketName,
				"key":     keyPath + "/terraform.tfstate",
				"region":  "us-west-2",
			},
		},
		remoteStateOut,
	)
}
