//go:build docker

package test_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	testFixtureOutputFromRemoteStateRustFS = "fixtures/output-from-remote-state-rustfs"
	testFixtureStackDepsStackMockRustFS    = "fixtures/stacks/stack-deps-stack-mock-rustfs"
)

func TestRustFSOutputFromRemoteState(t *testing.T) {
	rustfsAddr := setupRustFS(t)

	// RustFS default credentials
	t.Setenv("AWS_ACCESS_KEY_ID", "rustfsadmin")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "rustfsadmin")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputFromRemoteStateRustFS)

	rootTerragruntConfigPath := filepath.Join(
		tmpEnvPath,
		testFixtureOutputFromRemoteStateRustFS,
		"root.hcl",
	)
	helpers.CopyAndFillMapPlaceholders(
		t,
		rootTerragruntConfigPath,
		rootTerragruntConfigPath,
		map[string]string{
			"__FILL_IN_BUCKET_NAME__": s3BucketName,
			"__FILL_IN_S3_ENDPOINT__": rustfsAddr,
		},
	)

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

// TestRustFSStackDependencyMockOutputs covers stack dependency mock resolution against a live S3
// API, where a unit that hasn't been applied yet fails with a real NoSuchKey. It pins that a
// map-typed mock_outputs resolves for such a unit, and that a unit is never dropped silently from
// the aggregated stack outputs: neither when mock_outputs_allowed_terraform_commands rules its mocks
// out for the current command, nor when mock_outputs can't be keyed by unit name at all.
func TestRustFSStackDependencyMockOutputs(t *testing.T) {
	rustfsAddr := setupRustFS(t)

	// RustFS default credentials
	t.Setenv("AWS_ACCESS_KEY_ID", "rustfsadmin")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "rustfsadmin")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsStackMockRustFS)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsStackMockRustFS)

	rootConfigPath := filepath.Join(gitPath, "root.hcl")
	helpers.CopyAndFillMapPlaceholders(
		t,
		rootConfigPath,
		rootConfigPath,
		map[string]string{
			"__FILL_IN_BUCKET_NAME__": s3BucketName,
			"__FILL_IN_S3_ENDPOINT__": rustfsAddr,
		},
	)

	// The networking stack sources its units via get_repo_root(), so the fixture copy must be a git repo.
	helpers.CreateGitRepo(t, gitPath)

	rootPath := filepath.Join(gitPath, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	stackPath := filepath.Join(rootPath, ".terragrunt-stack")
	vpcPath := filepath.Join(stackPath, "networking", ".terragrunt-stack", "vpc")

	// Applying vpc creates the bucket, so the subnets unit that follows is missing a key rather than
	// a bucket. Only the former is treated as "not applied yet".
	helpers.RunTerragrunt(
		t,
		"terragrunt apply --backend-bootstrap --auto-approve --non-interactive --working-dir "+vpcPath,
	)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --dependency-fetch-output-from-state --backend-bootstrap --non-interactive --working-dir "+filepath.Join(
			stackPath,
			"app",
		),
	)
	require.NoError(
		t,
		err,
		"a map-typed mock_outputs must resolve for the stack unit with no state; stderr=%s",
		stderr,
	)
	assert.Contains(t, stdout, "real-vpc-id", "the applied unit must resolve to its real output")
	assert.Contains(t, stdout, "mock-subnet-id", "the unapplied unit must resolve to its mock")

	strictPath := filepath.Join(stackPath, "strict")

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --dependency-fetch-output-from-state --backend-bootstrap --non-interactive --working-dir "+strictPath,
	)

	var fetchErr config.StackUnitOutputFetchError

	require.ErrorAs(
		t,
		err,
		&fetchErr,
		"plan is not in mock_outputs_allowed_terraform_commands, so the unit with no state must not be dropped silently",
	)
	assert.Equal(t, "subnets", fetchErr.UnitName)

	_, stderr, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt validate --dependency-fetch-output-from-state --backend-bootstrap --non-interactive --working-dir "+strictPath,
	)
	require.NoError(
		t,
		err,
		"validate is in mock_outputs_allowed_terraform_commands, so the mock must stand in; stderr=%s",
		stderr,
	)

	malformedPath := filepath.Join(stackPath, "malformed")

	_, stderr, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --dependency-fetch-output-from-state --backend-bootstrap --non-interactive --working-dir "+malformedPath,
	)

	var mockTypeErr config.StackMockOutputsTypeError

	require.ErrorAs(
		t,
		err,
		&mockTypeErr,
		"mock_outputs that can't be keyed by unit name must fail rather than drop the unit; stderr=%s",
		stderr,
	)
	assert.Equal(t, "networking", mockTypeErr.DependencyName)
}

func setupRustFS(t *testing.T) string {
	t.Helper()

	_, addr := helpers.RunContainer(
		t,
		"rustfs/rustfs:1.0.0-alpha.90@sha256:0725587f6fcca83c1898f321424327d6e6da5e01ea20382905dd258ed5af3be4",
		9000,
		testcontainers.WithCmd("/data"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Starting:"),
		),
	)

	return addr
}
