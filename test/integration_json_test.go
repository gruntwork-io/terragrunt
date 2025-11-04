package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureRenderJSONMetadata    = "fixtures/render-json-metadata"
	testFixtureRenderJSONMockOutputs = "fixtures/render-json-mock-outputs"
	testFixtureRenderJSONInputs      = "fixtures/render-json-inputs"
)

func TestRenderJsonAttributesMetadata(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRenderJSONMetadata)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMetadata, "attributes")

	terragruntHCL := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMetadata, "attributes", "terragrunt.hcl")

	var expectedMetadata = map[string]any{
		"found_in_file": terragruntHCL,
	}

	jsonOut := filepath.Join(tmpDir, "terragrunt.rendered.json")

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render --json -w --with-metadata --non-interactive --log-level trace --working-dir %s  --out %s", tmpDir, jsonOut))

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJSON = map[string]any{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJSON))

	var (
		inputs         = renderedJSON[config.MetadataInputs]
		expectedInputs = map[string]any{"name": map[string]any{"metadata": expectedMetadata, "value": "us-east-1-bucket"}, "region": map[string]any{"metadata": expectedMetadata, "value": "us-east-1"}}
	)

	assert.True(t, reflect.DeepEqual(expectedInputs, inputs))

	var (
		locals         = renderedJSON[config.MetadataLocals]
		expectedLocals = map[string]any{"aws_region": map[string]any{"metadata": expectedMetadata, "value": "us-east-1"}}
	)

	assert.True(t, reflect.DeepEqual(expectedLocals, locals))

	var (
		downloadDir        = renderedJSON[config.MetadataDownloadDir]
		expecteDownloadDir = map[string]any{"metadata": expectedMetadata, "value": "/tmp"}
	)

	assert.True(t, reflect.DeepEqual(expecteDownloadDir, downloadDir))

	var iamAssumeRoleDuration = renderedJSON[config.MetadataIamAssumeRoleDuration]

	expectedIamAssumeRoleDuration := map[string]any{
		"metadata": expectedMetadata,
		"value":    float64(666),
	}
	assert.True(t, reflect.DeepEqual(expectedIamAssumeRoleDuration, iamAssumeRoleDuration))

	var iamAssumeRoleName = renderedJSON[config.MetadataIamAssumeRoleSessionName]

	expectedIamAssumeRoleName := map[string]any{
		"metadata": expectedMetadata,
		"value":    "qwe",
	}
	assert.True(t, reflect.DeepEqual(expectedIamAssumeRoleName, iamAssumeRoleName))

	var iamRole = renderedJSON[config.MetadataIamRole]

	expectedIamRole := map[string]any{
		"metadata": expectedMetadata,
		"value":    "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME",
	}
	assert.True(t, reflect.DeepEqual(expectedIamRole, iamRole))

	var preventDestroy = renderedJSON[config.MetadataPreventDestroy]

	expectedPreventDestroy := map[string]any{
		"metadata": expectedMetadata,
		"value":    true,
	}
	assert.True(t, reflect.DeepEqual(expectedPreventDestroy, preventDestroy))

	var terraformBinary = renderedJSON[config.MetadataTerraformBinary]

	expectedTerraformBinary := map[string]any{
		"metadata": expectedMetadata,
		"value":    wrappedBinary(),
	}
	assert.True(t, reflect.DeepEqual(expectedTerraformBinary, terraformBinary), "expected: %v, got: %v", expectedTerraformBinary, terraformBinary)

	var terraformVersionConstraint = renderedJSON[config.MetadataTerraformVersionConstraint]

	expectedTerraformVersionConstraint := map[string]any{
		"metadata": expectedMetadata,
		"value":    ">= 0.11",
	}
	assert.True(t, reflect.DeepEqual(expectedTerraformVersionConstraint, terraformVersionConstraint))
}

func TestRenderJsonWithInputsNotExistingOutput(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRenderJSONInputs)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	dependencyPath := util.JoinPath(tmpEnvPath, testFixtureRenderJSONInputs, "dependency")
	appPath := util.JoinPath(tmpEnvPath, testFixtureRenderJSONInputs, "app")

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+dependencyPath)
	helpers.RunTerragrunt(t, "terragrunt render --json -w --with-metadata --non-interactive --log-level trace --working-dir "+appPath)

	jsonOut := filepath.Join(appPath, "terragrunt.rendered.json")

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJSON = map[string]any{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJSON))

	var includeMetadata = map[string]any{
		"found_in_file": util.JoinPath(appPath, "terragrunt.hcl"),
	}

	var (
		inputs         = renderedJSON[config.MetadataInputs]
		expectedInputs = map[string]any{"static_value": map[string]any{"metadata": includeMetadata, "value": "static_value"}, "value": map[string]any{"metadata": includeMetadata, "value": "output_value"}, "not_existing_value": map[string]any{"metadata": includeMetadata, "value": ""}}
	)

	assert.True(t, reflect.DeepEqual(expectedInputs, inputs))
}

func TestRenderJsonWithMockOutputs(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRenderJSONMockOutputs)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMockOutputs, "app")

	var expectedMetadata = map[string]any{
		"found_in_file": util.JoinPath(tmpDir, "terragrunt.hcl"),
	}

	jsonOut := filepath.Join(tmpDir, "terragrunt.rendered.json")

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render --json -w --with-metadata --non-interactive --log-level trace --working-dir %s  --out %s", tmpDir, jsonOut))

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJSON = map[string]any{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJSON))

	dependency := renderedJSON[config.MetadataDependency]

	var expectedDependency = map[string]any{
		"module": map[string]any{
			"metadata": expectedMetadata,
			"value": map[string]any{
				"config_path": "../dependency",
				"enabled":     nil,
				"mock_outputs": map[string]any{
					"bastion_host_security_group_id": "123",
					"security_group_id":              "sg-abcd1234",
				},
				"mock_outputs_allowed_terraform_commands": [1]string{"validate"},
				"mock_outputs_merge_strategy_with_state":  nil,
				"mock_outputs_merge_with_state":           nil,
				"name":                                    "module",
				"outputs":                                 nil,
				"inputs":                                  nil,
				"skip":                                    nil,
			},
		},
	}

	serializedDependency, err := json.Marshal(dependency)
	require.NoError(t, err)

	serializedExpectedDependency, err := json.Marshal(expectedDependency)
	require.NoError(t, err)
	assert.Equal(t, string(serializedExpectedDependency), string(serializedDependency))
}

func TestRenderJsonMetadataIncludes(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRenderJSONMetadata)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMetadata, "includes", "app")

	terragruntHcl := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMetadata, "includes", "app", "terragrunt.hcl")
	localsHcl := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMetadata, "includes", "app", "locals.hcl")
	inputHcl := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMetadata, "includes", "app", "inputs.hcl")
	generateHcl := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMetadata, "includes", "app", "generate.hcl")
	commonHcl := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMetadata, "includes", "common", "common.hcl")

	var (
		terragruntMetadata = map[string]any{"found_in_file": terragruntHcl}
		localsMetadata     = map[string]any{"found_in_file": localsHcl}
		inputMetadata      = map[string]any{"found_in_file": inputHcl}
		generateMetadata   = map[string]any{"found_in_file": generateHcl}
		commonMetadata     = map[string]any{"found_in_file": commonHcl}
	)

	jsonOut := filepath.Join(tmpDir, "terragrunt.rendered.json")

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render --json -w --with-metadata --non-interactive --log-level trace --working-dir %s  --out %s", tmpDir, jsonOut))

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJSON = map[string]any{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJSON))

	var (
		inputs         = renderedJSON[config.MetadataInputs]
		expectedInputs = map[string]any{"content": map[string]any{"metadata": localsMetadata, "value": "test"}, "qwe": map[string]any{"metadata": inputMetadata, "value": "123"}}
	)

	assert.True(t, reflect.DeepEqual(expectedInputs, inputs))

	var (
		locals         = renderedJSON[config.MetadataLocals]
		expectedLocals = map[string]any{"abc": map[string]any{"metadata": terragruntMetadata, "value": "xyz"}}
	)

	assert.True(t, reflect.DeepEqual(expectedLocals, locals))

	var (
		generate         = renderedJSON[config.MetadataGenerateConfigs]
		expectedGenerate = map[string]any{"provider": map[string]any{"metadata": generateMetadata, "value": map[string]any{"comment_prefix": "# ", "contents": "# test\n", "disable_signature": false, "disable": false, "if_exists": "overwrite", "if_disabled": "skip", "hcl_fmt": nil, "path": "provider.tf"}}}
	)

	// compare fields by serialization in json since map from "value" field is not deterministic
	serializedGenerate, err := json.Marshal(generate)
	require.NoError(t, err)

	serializedExpectedGenerate, err := json.Marshal(expectedGenerate)
	require.NoError(t, err)

	assert.Equal(t, string(serializedExpectedGenerate), string(serializedGenerate))

	var (
		remoteState         = renderedJSON[config.MetadataRemoteState]
		expectedRemoteState = map[string]any{"metadata": commonMetadata, "value": map[string]any{"backend": "s3", "disable_dependency_optimization": false, "disable_init": false, "generate": nil, "config": map[string]any{"bucket": "mybucket", "key": "path/to/my/key", "region": "us-east-1"}, "encryption": nil}}
	)

	// compare fields by serialization in json since map from "value" field is not deterministic
	serializedRemoteState, err := json.Marshal(remoteState)
	require.NoError(t, err)

	serializedExpectedRemoteState, err := json.Marshal(expectedRemoteState)
	require.NoError(t, err)

	assert.Equal(t, string(serializedExpectedRemoteState), string(serializedRemoteState))
}

func TestRenderJsonMetadataDependency(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRenderJSONMetadata)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMetadata, "dependency", "app")

	terragruntHcl := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMetadata, "dependency", "app", "terragrunt.hcl")

	var terragruntMetadata = map[string]any{
		"found_in_file": terragruntHcl,
	}

	jsonOut := filepath.Join(tmpDir, "terragrunt.rendered.json")

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render --json -w --with-metadata --non-interactive --log-level trace --working-dir %s  --out %s", tmpDir, jsonOut))

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJSON = map[string]any{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJSON))

	var dependency = renderedJSON[config.MetadataDependency]

	var expectedDependency = map[string]any{
		"dep": map[string]any{
			"metadata": terragruntMetadata,
			"value": map[string]any{
				"config_path": "../dependency",
				"mock_outputs": map[string]any{
					"test": "value",
				},
				"mock_outputs_allowed_terraform_commands": nil,
				"mock_outputs_merge_strategy_with_state":  nil,
				"mock_outputs_merge_with_state":           nil,
				"name":                                    "dep",
				"outputs":                                 nil,
				"inputs":                                  nil,
				"skip":                                    nil,
				"enabled":                                 nil,
			},
		},
		"dep2": map[string]any{
			"metadata": terragruntMetadata,
			"value": map[string]any{
				"config_path": "../dependency2",
				"enabled":     nil,
				"mock_outputs": map[string]any{
					"test2": "value2",
				},
				"mock_outputs_allowed_terraform_commands": nil,
				"mock_outputs_merge_strategy_with_state":  nil,
				"mock_outputs_merge_with_state":           nil,
				"name":                                    "dep2",
				"outputs":                                 nil,
				"inputs":                                  nil,
				"skip":                                    nil,
			},
		},
	}

	// compare fields by serialization in json since map from "value" field is not deterministic
	serializedDependency, err := json.Marshal(dependency)
	require.NoError(t, err)

	serializedExpectedDependency, err := json.Marshal(expectedDependency)
	require.NoError(t, err)

	assert.Equal(t, string(serializedExpectedDependency), string(serializedDependency))
}

func TestRenderJsonMetadataTerraform(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRenderJSONMetadata)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMetadata, "terraform-remote-state", "app")

	commonHcl := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMetadata, "terraform-remote-state", "common", "terraform.hcl")
	remoteStateHcl := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMetadata, "terraform-remote-state", "common", "remote_state.hcl")

	var (
		terragruntMetadata = map[string]any{"found_in_file": commonHcl}
		remoteMetadata     = map[string]any{"found_in_file": remoteStateHcl}
	)

	jsonOut := filepath.Join(tmpDir, "terragrunt.rendered.json")

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render --json -w --with-metadata --non-interactive --log-level trace --working-dir %s  --out %s", tmpDir, jsonOut))

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJSON = map[string]any{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJSON))

	var (
		terraform         = renderedJSON[config.MetadataTerraform]
		expectedTerraform = map[string]any{"metadata": terragruntMetadata, "value": map[string]any{"after_hook": map[string]any{}, "before_hook": map[string]any{}, "error_hook": map[string]any{}, "extra_arguments": map[string]any{}, "include_in_copy": nil, "exclude_from_copy": nil, "source": "../terraform", "copy_terraform_lock_file": nil}}
	)

	// compare fields by serialization in json since map from "value" field is not deterministic
	serializedTerraform, err := json.Marshal(terraform)
	require.NoError(t, err)

	serializedExpectedTerraform, err := json.Marshal(expectedTerraform)
	require.NoError(t, err)

	assert.Equal(t, string(serializedExpectedTerraform), string(serializedTerraform))

	var (
		remoteState         = renderedJSON[config.MetadataRemoteState]
		expectedRemoteState = map[string]any{"metadata": remoteMetadata, "value": map[string]any{"backend": "s3", "config": map[string]any{"bucket": "mybucket", "key": "path/to/my/key", "region": "us-east-1"}, "encryption": nil, "disable_dependency_optimization": false, "disable_init": false, "generate": nil}}
	)

	// compare fields by serialization in json since map from "value" field is not deterministic
	serializedRemoteState, err := json.Marshal(remoteState)
	require.NoError(t, err)

	serializedExpectedRemoteState, err := json.Marshal(expectedRemoteState)
	require.NoError(t, err)

	assert.Equal(t, string(serializedExpectedRemoteState), string(serializedRemoteState))
}

func TestRenderJsonMetadataDependencyModulePrefix(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRenderJSONMetadata)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, testFixtureRenderJSONMetadata, "dependency", "app")

	helpers.RunTerragrunt(t, "terragrunt render --json -w --with-metadata --non-interactive --log-level trace --working-dir "+tmpDir)
}

func TestRenderJsonDependentModulesMetadataTerraform(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDestroyWarning)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, testFixtureDestroyWarning, "vpc")

	jsonOut := filepath.Join(tmpDir, "terragrunt.rendered.json")
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render --json -w --with-metadata --non-interactive --log-level trace --working-dir %s --out %s", tmpDir, jsonOut))

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJSON = map[string]map[string]any{}

	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJSON))

	dependentModules := renderedJSON[config.MetadataDependentModules]["value"].([]any)
	// check if value list contains app-v1 and app-v2
	assert.Contains(t, dependentModules, util.JoinPath(tmpEnvPath, testFixtureDestroyWarning, "app-v1"))
	assert.Contains(t, dependentModules, util.JoinPath(tmpEnvPath, testFixtureDestroyWarning, "app-v2"))
}

func TestTerragruntRenderJsonHelp(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHooksInitOnceWithSourceNoBackend)
	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/hooks/init-once")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksInitOnceWithSourceNoBackend)

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt render --help --non-interactive --working-dir "+rootPath, &showStdout, &showStderr)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")

	output := showStdout.String()

	assert.Contains(t, output, "terragrunt render")
	assert.Contains(t, output, "--with-metadata")
}

func TestRenderJsonDependentModulesTerraform(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDestroyWarning)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, testFixtureDestroyWarning, "vpc")

	jsonOut := filepath.Join(tmpDir, "terragrunt.rendered.json")
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render --json -w --non-interactive --log-level trace --working-dir %s --out %s", tmpDir, jsonOut))

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJSON = map[string]any{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJSON))

	var dependentModules = renderedJSON[config.MetadataDependentModules].([]any)
	// check if value list contains app-v1 and app-v2
	assert.Contains(t, dependentModules, util.JoinPath(tmpEnvPath, testFixtureDestroyWarning, "app-v1"))
	assert.Contains(t, dependentModules, util.JoinPath(tmpEnvPath, testFixtureDestroyWarning, "app-v2"))
}

func TestRenderJsonDisableDependentModulesTerraform(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDestroyWarning)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, testFixtureDestroyWarning, "vpc")

	jsonOut := filepath.Join(tmpDir, "terragrunt.rendered.json")
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render --json -w --disable-dependent-modules --non-interactive --log-level trace --working-dir %s --out %s", tmpDir, jsonOut))

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJSON = map[string]any{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJSON))

	_, found := renderedJSON[config.MetadataDependentModules].([]any)
	assert.False(t, found)
}
