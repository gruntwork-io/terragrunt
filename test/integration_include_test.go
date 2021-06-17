package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	includeDeepFixturePath      = "fixture-include-deep/"
	includeDeepFixtureChildPath = "child"
	includeFixturePath          = "fixture-include/"
	includeShallowFixturePath   = "stage/my-app"
	includeNoMergeFixturePath   = "qa/my-app"
	includeExposeFixturePath    = "fixture-include-expose/"
	includeChildFixturePath     = "child"
)

func TestTerragruntWorksWithIncludeLocals(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(includeExposeFixturePath, includeChildFixturePath)
	cleanupTerraformFolder(t, childPath)
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", childPath))

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", childPath), &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	assert.Equal(t, "us-west-1-test", outputs["region"].Value.(string))
}

func TestTerragruntWorksWithIncludeShallowMerge(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(includeFixturePath, includeShallowFixturePath)
	cleanupTerraformFolder(t, childPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, includeFixturePath, includeShallowFixturePath, s3BucketName, config.DefaultTerragruntConfigPath, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))
	validateIncludeRemoteStateReflection(t, s3BucketName, includeShallowFixturePath, tmpTerragruntConfigPath, childPath)
}

func TestTerragruntWorksWithIncludeNoMerge(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(includeFixturePath, includeNoMergeFixturePath)
	cleanupTerraformFolder(t, childPath)

	// We deliberately pick an s3 bucket name that is invalid, as we don't expect to create this s3 bucket.
	s3BucketName := "__INVALID_NAME__"

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, includeFixturePath, includeNoMergeFixturePath, s3BucketName, config.DefaultTerragruntConfigPath, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))
	validateIncludeRemoteStateReflection(t, s3BucketName, includeNoMergeFixturePath, tmpTerragruntConfigPath, childPath)
}

func TestTerragruntWorksWithIncludeDeepMerge(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(includeDeepFixturePath, "child")
	cleanupTerraformFolder(t, childPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", childPath))

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", childPath), &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))

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

func validateIncludeRemoteStateReflection(t *testing.T, s3BucketName string, keyPath string, configPath string, workingDir string) {
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-config %s --terragrunt-working-dir %s", configPath, workingDir), &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	remoteStateOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["reflect"].Value.(string)), &remoteStateOut))
	assert.Equal(
		t,
		remoteStateOut,
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
	)
}
