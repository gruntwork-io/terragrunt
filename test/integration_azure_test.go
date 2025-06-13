//go:build azure

package test_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/azurehelper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureAzureBackend               = "./fixtures/azure-backend"
	testFixtureAzureOutputFromRemoteState = "./fixtures/azure-output-from-remote-state"
)

// TestCase represents the test case data without the check function.
type TestCase struct {
	name          string
	args          string
	containerName string
}

func TestAzureRMBootstrapBackend(t *testing.T) {
	t.Parallel()

	t.Log("Starting TestAzureRMBootstrapBackend")

	testCases := []struct {
		TestCase
		checkExpectedResultFn func(t *testing.T, err error, output string, containerName string, rootPath string, tc *TestCase)
	}{
		{
			TestCase: TestCase{
				name:          "delete backend command",
				args:          "backend delete --force",
				containerName: "terragrunt-test-container-" + strings.ToLower(helpers.UniqueID()),
			},
			checkExpectedResultFn: func(t *testing.T, err error, output string, containerName string, rootPath string, tc *TestCase) {
				t.Helper()

				// In delete case, not finding the container is acceptable
				if strings.Contains(output, "ContainerNotFound") {
					return
				}

				// For thoroughness, let's try bootstrapping and then deleting
				azureCfg := helpers.GetAzureStorageTestConfig(t)
				azureCfg.ContainerName = containerName

				// Bootstrap the backend first
				bootstrapOutput, bootstrapErr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt backend bootstrap --backend-bootstrap --non-interactive --log-level debug --log-format key-value --working-dir "+rootPath)
				require.NoError(t, err, "Bootstrap command failed: %v\nOutput: %s\nError: %s", err, bootstrapOutput, bootstrapErr)

				opts, err := options.NewTerragruntOptionsForTest("")
				require.NoError(t, err)

				client, err := azurehelper.CreateBlobServiceClient(
					logger.CreateLogger(),
					opts,
					map[string]interface{}{
						"storage_account_name": azureCfg.StorageAccountName,
						"container_name":       containerName,
						"use_azuread_auth":    true,
					},
				)
				require.NoError(t, err)

				// Verify container exists after bootstrap
				exists, err := client.ContainerExists(context.Background(), containerName)
				require.NoError(t, err)
				assert.True(t, exists, "Container should exist after bootstrap")

				// Create and verify test state file
				data := []byte("{}")
				err = client.UploadBlob(context.Background(), logger.CreateLogger(), containerName, "unit1/terraform.tfstate", data)
				require.NoError(t, err, "Failed to create test state file")

				stateKey := "unit1/terraform.tfstate"
				_, err = client.GetObject(context.Background(), &azurehelper.GetObjectInput{
					Bucket: &containerName,
					Key:    &stateKey,
				})
				require.NoError(t, err, "State file should exist after creation")

				// Now run the delete command again (will be run by test runner)
				deleteOutput, deleteErr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt backend delete --force --non-interactive --log-level debug --log-format key-value --working-dir "+rootPath)
				require.NoError(t, err, "Delete command failed: %v\nOutput: %s\nError: %s", err, deleteOutput, deleteErr)

				// Verify container is deleted with retries
				var containerExists bool
				maxRetries := 5
				for i := 0; i < maxRetries; i++ {
					exists, err = client.ContainerExists(context.Background(), containerName)
					require.NoError(t, err)
					if !exists {
						containerExists = false
						break
					}
					time.Sleep(3 * time.Second)
				}
				assert.False(t, containerExists, "Container should not exist after delete")
			},
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureAzureBackend)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAzureBackend)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureAzureBackend)
			commonConfigPath := util.JoinPath(rootPath, "common.hcl")

			azureCfg := helpers.GetAzureStorageTestConfig(t)

			defer func() {
				// Clean up the destination container
				azureCfg.ContainerName = tc.containerName
				helpers.CleanupAzureContainer(t, azureCfg)
			}()

			// Set up common configuration parameters
			azureParams := map[string]string{
				"__FILL_IN_STORAGE_ACCOUNT_NAME__": azureCfg.StorageAccountName,
				"__FILL_IN_SUBSCRIPTION_ID__":      os.Getenv("AZURE_SUBSCRIPTION_ID"),
				"__FILL_IN_CONTAINER_NAME__":       tc.containerName,
			}

			// Set up the common configuration
			helpers.CopyTerragruntConfigAndFillProviderPlaceholders(t,
				commonConfigPath,
				commonConfigPath,
				azureParams,
				azureCfg.Location)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt "+tc.args+" --all --non-interactive --log-level debug --log-format key-value --strict-control require-explicit-bootstrap --working-dir "+rootPath)

			tc.checkExpectedResultFn(t, err, stdout+stderr, tc.containerName, rootPath, &tc.TestCase)
		})
	}
}

// TestAzureOutputFromRemoteState tests the ability to get outputs from remote state stored in Azure Storage.
func TestAzureOutputFromRemoteState(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAzureOutputFromRemoteState)

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureAzureOutputFromRemoteState)

	azureCfg := helpers.GetAzureStorageTestConfig(t)

	// Fill in Azure configuration
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAzureOutputFromRemoteState)
	rootTerragruntConfigPath := util.JoinPath(rootPath, "root.hcl")
	containerName := "terragrunt-test-container-" + strings.ToLower(helpers.UniqueID())

	// Set up Azure configuration parameters
	azureParams := map[string]string{
		"__FILL_IN_STORAGE_ACCOUNT_NAME__": azureCfg.StorageAccountName,
		"__FILL_IN_SUBSCRIPTION_ID__":      os.Getenv("AZURE_SUBSCRIPTION_ID"),
		"__FILL_IN_CONTAINER_NAME__":       containerName,
	}

	// Set up the root configuration
	helpers.CopyTerragruntConfigAndFillProviderPlaceholders(t,
		rootTerragruntConfigPath,
		rootTerragruntConfigPath,
		azureParams,
		azureCfg.Location)

	defer func() {
		// Clean up the destination container
		azureCfg.ContainerName = containerName
		helpers.CleanupAzureContainer(t, azureCfg)
	}()

	// Run terragrunt for app3, app2, and app1 in that order (dependencies first)
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+fmt.Sprintf("%s/app3", environmentPath))
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+fmt.Sprintf("%s/app2", environmentPath))
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+fmt.Sprintf("%s/app1", environmentPath))

	// Now check the outputs to make sure the remote state dependencies work
	app1OutputCmd := "terragrunt output -no-color -json --non-interactive --working-dir " + fmt.Sprintf("%s/app1", environmentPath)
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, app1OutputCmd, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	// Verify outputs from app1
	assert.Equal(t, outputs["combined_output"].Value, `app1 output with app2 output with app3 output and app3 output`)
}
