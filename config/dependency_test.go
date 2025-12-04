package config_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/gruntwork-io/go-commons/env"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

func TestDecodeDependencyBlockMultiple(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../vpc"
}

dependency "sql" {
  config_path = "../sql"
}
`
	filename := config.DefaultTerragruntConfigPath
	file, err := hclparse.NewParser().ParseFromString(cfg, filename)
	require.NoError(t, err)

	decoded := config.TerragruntDependency{}
	require.NoError(t, file.Decode(&decoded, &hcl.EvalContext{}))

	assert.Len(t, decoded.Dependencies, 2)
	assert.Equal(t, "vpc", decoded.Dependencies[0].Name)
	assert.Equal(t, cty.StringVal("../vpc"), decoded.Dependencies[0].ConfigPath)
	assert.Equal(t, "sql", decoded.Dependencies[1].Name)
	assert.Equal(t, cty.StringVal("../sql"), decoded.Dependencies[1].ConfigPath)
}

func TestDecodeNoDependencyBlock(t *testing.T) {
	t.Parallel()

	cfg := `
locals {
  path = "../vpc"
}
`
	filename := config.DefaultTerragruntConfigPath
	file, err := hclparse.NewParser().ParseFromString(cfg, filename)
	require.NoError(t, err)

	decoded := config.TerragruntDependency{}
	require.NoError(t, file.Decode(&decoded, &hcl.EvalContext{}))
	assert.Empty(t, decoded.Dependencies)
}

func TestDecodeDependencyNoLabelIsError(t *testing.T) {
	t.Parallel()

	cfg := `
dependency {
  config_path = "../vpc"
}
`
	filename := config.DefaultTerragruntConfigPath
	file, err := hclparse.NewParser().ParseFromString(cfg, filename)
	require.NoError(t, err)

	decoded := config.TerragruntDependency{}
	require.Error(t, file.Decode(&decoded, &hcl.EvalContext{}))
}

func TestDecodeDependencyMockOutputs(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "hitchhiker" {
  config_path = "../answers"
  mock_outputs = {
    the_answer = 42
  }
  mock_outputs_allowed_terraform_commands = ["validate", "apply"]
}
`
	filename := config.DefaultTerragruntConfigPath
	file, err := hclparse.NewParser().ParseFromString(cfg, filename)
	require.NoError(t, err)

	decoded := config.TerragruntDependency{}
	require.NoError(t, file.Decode(&decoded, &hcl.EvalContext{}))

	assert.Len(t, decoded.Dependencies, 1)
	dependency := decoded.Dependencies[0]
	assert.Equal(t, "hitchhiker", dependency.Name)
	assert.Equal(t, cty.StringVal("../answers"), dependency.ConfigPath)

	ctyValueDefault := dependency.MockOutputs
	assert.NotNil(t, ctyValueDefault)

	var actualDefault struct {
		TheAnswer int `cty:"the_answer"`
	}
	require.NoError(t, gocty.FromCtyValue(*ctyValueDefault, &actualDefault))
	assert.Equal(t, 42, actualDefault.TheAnswer)

	defaultAllowedCommands := dependency.MockOutputsAllowedTerraformCommands
	assert.NotNil(t, defaultAllowedCommands)
	assert.Equal(t, []string{"validate", "apply"}, *defaultAllowedCommands)
}
func TestParseDependencyBlockMultiple(t *testing.T) {
	t.Parallel()

	filename := "../test/fixtures/regressions/multiple-dependency-load-sync/main/terragrunt.hcl"
	ctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), mockOptionsForTestWithConfigPath(t, filename))
	opts, err := options.NewTerragruntOptionsForTest(filename)
	require.NoError(t, err)

	ctx.TerragruntOptions = opts
	ctx.TerragruntOptions.FetchDependencyOutputFromState = true
	ctx.TerragruntOptions.Env = env.Parse(os.Environ())
	tfConfig, err := config.ParseConfigFile(ctx, logger.CreateLogger(), filename, nil)
	require.NoError(t, err)
	assert.Len(t, tfConfig.TerragruntDependencies, 2)
	assert.Equal(t, "dependency_1", tfConfig.TerragruntDependencies[0].Name)
	assert.Equal(t, "dependency_2", tfConfig.TerragruntDependencies[1].Name)
}

func TestDisabledDependency(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "ec2" {
  config_path = "../ec2"
  enabled    = false
}
dependency "vpc" {
  config_path = "../vpc"
}
`
	filename := config.DefaultTerragruntConfigPath
	file, err := hclparse.NewParser().ParseFromString(cfg, filename)
	require.NoError(t, err)

	decoded := config.TerragruntDependency{}
	require.NoError(t, file.Decode(&decoded, &hcl.EvalContext{}))
	assert.Len(t, decoded.Dependencies, 2)
}

func TestDirectStateAccessAzurerm(t *testing.T) {
	t.Parallel()

	// Skip test if we're not running on Azure environment
	if os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT") == "" {
		t.Skip("Skipping Azure test as TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT is not set")
	}

	// Create a mock state file with known outputs
	stateOutputs := `{
        "version": 4,
        "terraform_version": "1.3.7",
        "serial": 1,
        "lineage": "12345678-1234-1234-1234-123456789012",
        "outputs": {
            "test_output": {
                "value": "azure_test_value",
                "type": "string"
            }
        },
        "resources": []
    }`

	// Get Azure test config
	storageAccount := os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")
	uniqueID := strconv.FormatInt(time.Now().UnixNano(), 10)
	containerName := "terragrunt-test-container-" + strings.ToLower(uniqueID)
	blobName := "terraform.tfstate"

	// Create a logger for testing
	testLogger := logger.CreateLogger()

	// Create options
	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Setup remote state config
	backendConfig := map[string]interface{}{
		"storage_account_name": storageAccount,
		"container_name":       containerName,
		"key":                  blobName,
		"use_azuread_auth":     true,
	}

	remoteStateConfig := &remotestate.Config{
		BackendName:   "azurerm",
		BackendConfig: backendConfig,
	}

	remoteState := remotestate.New(remoteStateConfig)

	// Create Azure client - use t.Context() instead of context.Background()
	ctx := t.Context()
	client, err := azurehelper.CreateBlobServiceClient(ctx, testLogger, terragruntOptions, backendConfig)
	require.NoError(t, err)

	// Setup - Create container and upload state file
	err = client.CreateContainerIfNecessary(ctx, testLogger, containerName)
	require.NoError(t, err)

	defer func() {
		err = client.DeleteContainer(ctx, testLogger, containerName)
		require.NoError(t, err)
	}()

	err = client.UploadBlob(ctx, testLogger, containerName, blobName, []byte(stateOutputs))
	require.NoError(t, err)

	// Test direct state access
	jsonBytes, err := getTerragruntOutputJSONFromRemoteStateAzurerm(ctx, testLogger, terragruntOptions, remoteState)
	require.NoError(t, err)

	// Parse and verify outputs
	var stateFileObj map[string]interface{}
	err = json.Unmarshal(jsonBytes, &stateFileObj)
	require.NoError(t, err)

	stateOutputsMap, outputsOk := stateFileObj["outputs"].(map[string]interface{})
	require.True(t, outputsOk, "outputs section not found in state file")

	// Verify the output value
	testOutputIface, testOutputOk := stateOutputsMap["test_output"]
	require.True(t, testOutputOk, "test_output not found in state file")

	testOutputMap, testOutputMapOk := testOutputIface.(map[string]interface{})
	require.True(t, testOutputMapOk, "test_output is not a map")

	valueIface, valueOk := testOutputMap["value"]
	require.True(t, valueOk, "value not found in test_output")

	valueStr, valueStrOk := valueIface.(string)
	require.True(t, valueStrOk, "value is not a string")
	assert.Equal(t, "azure_test_value", valueStr)
}

// getTerragruntOutputJSONFromRemoteStateAzurerm pulls the output directly from an Azure storage without calling Terraform
// This is the test version of the function from config/dependency.go
func getTerragruntOutputJSONFromRemoteStateAzurerm(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, remoteState *remotestate.RemoteState) ([]byte, error) {
	// Create Azure blob client from the configuration
	client, err := azurehelper.CreateBlobServiceClient(ctx, l, opts, remoteState.BackendConfig)
	if err != nil {
		return nil, err
	}

	// Extract required configuration values
	containerName := remoteState.BackendConfig["container_name"].(string)
	key := remoteState.BackendConfig["key"].(string)

	// Check if container exists
	exists, err := client.ContainerExists(ctx, containerName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.Errorf("Azure container %s does not exist", containerName)
	}

	// Get the state file blob content
	container := containerName
	keyPtr := &key
	containerPtr := &container
	input := &azurehelper.GetObjectInput{
		Container: containerPtr,
		Key:       keyPtr,
	}

	output, err := client.GetObject(ctx, input)
	if err != nil {
		return nil, errors.Errorf("error reading terraform state blob %s from container %s: %w", key, containerName, err)
	}

	defer output.Body.Close()
	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, errors.Errorf("error reading response body: %w", err)
	}

	return data, nil
}
