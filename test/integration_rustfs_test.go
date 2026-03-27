//go:build docker

package test_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	testFixtureOutputFromRemoteStateRustFS = "fixtures/output-from-remote-state-rustfs"
)

func TestRustFSOutputFromRemoteState(t *testing.T) { //nolint: paralleltest
	t.Skip("Skipping until integration in CI is resolved")

	rustfsAddr := setupRustFS(t)

	// RustFS default credentials
	t.Setenv("AWS_ACCESS_KEY_ID", "rustfsadmin")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "rustfsadmin")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputFromRemoteStateRustFS)

	rootTerragruntConfigPath := filepath.Join(tmpEnvPath, testFixtureOutputFromRemoteStateRustFS, "root.hcl")
	helpers.CopyAndFillMapPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, map[string]string{
		"__FILL_IN_BUCKET_NAME__": s3BucketName,
		"__FILL_IN_S3_ENDPOINT__": rustfsAddr,
	})

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputFromRemoteStateRustFS)

	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt run --backend-bootstrap --dependency-fetch-output-from-state "+
				"--non-interactive --working-dir %s/app1 -- apply -auto-approve",
			environmentPath,
		),
	)
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt run --backend-bootstrap --dependency-fetch-output-from-state "+
				"--non-interactive --working-dir %s/app3 -- apply -auto-approve",
			environmentPath,
		),
	)

	// Delete dependencies cached state to force fetching from remote state
	app1CacheDir := helpers.FindCacheWorkingDir(t, filepath.Join(environmentPath, "app1"))
	require.NotEmpty(t, app1CacheDir, "Cache directory for app1 should exist")
	require.NoError(t, os.Remove(filepath.Join(app1CacheDir, ".terraform/terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(app1CacheDir, ".terraform")))
	app3CacheDir := helpers.FindCacheWorkingDir(t, filepath.Join(environmentPath, "app3"))
	require.NotEmpty(t, app3CacheDir, "Cache directory for app3 should exist")
	require.NoError(t, os.Remove(filepath.Join(app3CacheDir, ".terraform/terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(app3CacheDir, ".terraform")))

	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt run --backend-bootstrap --dependency-fetch-output-from-state "+
				"--non-interactive --working-dir %s/app2 -- apply -auto-approve",
			environmentPath,
		),
	)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all output --backend-bootstrap --dependency-fetch-output-from-state --non-interactive --working-dir "+environmentPath,
	)
	require.NoError(t, err)

	assert.Contains(t, stdout, "app1 output")
	assert.Contains(t, stdout, "app2 output")
	assert.Contains(t, stdout, "app3 output")
	assert.NotContains(t, stderr, "terraform output -json")
	assert.NotContains(t, stderr, "tofu output -json")
}

func setupRustFS(t *testing.T) string {
	t.Helper()

	_, addr := helpers.RunContainer(t, "rustfs/rustfs:latest", 9000,
		testcontainers.WithCmd("/data"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Starting:"),
		),
	)

	return addr
}
