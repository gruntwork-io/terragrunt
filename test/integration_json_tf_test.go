//go:build tf

package test_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFRenderJsonAttributesMetadata(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRenderJSONMetadata)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := filepath.Join(tmpEnvPath, testFixtureRenderJSONMetadata, "attributes")

	terragruntHCL := filepath.Join(
		tmpEnvPath,
		testFixtureRenderJSONMetadata,
		"attributes",
		"terragrunt.hcl",
	)

	var expectedMetadata = map[string]any{
		"found_in_file": terragruntHCL,
	}

	jsonOut := filepath.Join(tmpDir, "terragrunt.rendered.json")

	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt render --json -w --with-metadata --non-interactive --working-dir %s  --out %s",
			tmpDir,
			jsonOut,
		),
	)

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJSON = map[string]any{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJSON))

	var (
		inputs         = renderedJSON[config.MetadataInputs]
		expectedInputs = map[string]any{
			"name":   map[string]any{"metadata": expectedMetadata, "value": "us-east-1-bucket"},
			"region": map[string]any{"metadata": expectedMetadata, "value": "us-east-1"},
		}
	)

	assert.True(t, reflect.DeepEqual(expectedInputs, inputs))

	var (
		locals         = renderedJSON[config.MetadataLocals]
		expectedLocals = map[string]any{
			"aws_region": map[string]any{"metadata": expectedMetadata, "value": "us-east-1"},
		}
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
		"value":    wrappedBinary(t.Context()),
	}
	assert.True(
		t,
		reflect.DeepEqual(expectedTerraformBinary, terraformBinary),
		"expected: %v, got: %v",
		expectedTerraformBinary,
		terraformBinary,
	)

	var terraformVersionConstraint = renderedJSON[config.MetadataTerraformVersionConstraint]

	expectedTerraformVersionConstraint := map[string]any{
		"metadata": expectedMetadata,
		"value":    ">= 0.11",
	}
	assert.True(
		t,
		reflect.DeepEqual(expectedTerraformVersionConstraint, terraformVersionConstraint),
	)
}

func TestTFRenderJsonWithInputsNotExistingOutput(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRenderJSONInputs)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	dependencyPath := filepath.Join(tmpEnvPath, testFixtureRenderJSONInputs, "dependency")
	appPath := filepath.Join(tmpEnvPath, testFixtureRenderJSONInputs, "app")

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+dependencyPath,
	)
	helpers.RunTerragrunt(
		t,
		"terragrunt render --json -w --with-metadata --non-interactive --working-dir "+appPath,
	)

	jsonOut := filepath.Join(appPath, "terragrunt.rendered.json")

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJSON = map[string]any{}
	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJSON))

	var includeMetadata = map[string]any{
		"found_in_file": filepath.Join(appPath, "terragrunt.hcl"),
	}

	var (
		inputs         = renderedJSON[config.MetadataInputs]
		expectedInputs = map[string]any{
			"static_value": map[string]any{
				"metadata": includeMetadata,
				"value":    "static_value",
			},
			"value": map[string]any{
				"metadata": includeMetadata,
				"value":    "output_value",
			},
			"not_existing_value": map[string]any{"metadata": includeMetadata, "value": ""},
		}
	)

	assert.True(t, reflect.DeepEqual(expectedInputs, inputs))
}
