//go:build docker

package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/writer"
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

// TestRustFSDependencyOutputOptimization pins how a unit reads its
// dependency's outputs when the dependency's own dependency has no state. The
// live unit depends on dep, and dep depends on deepdep. Each case deletes
// deepdep's state object and checks which commands the live unit spawns.
func TestRustFSDependencyOutputOptimization(t *testing.T) {
	rustfsAddr := setupRustFS(t)

	t.Setenv("AWS_ACCESS_KEY_ID", "rustfsadmin")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "rustfsadmin")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")

	testCases := []struct {
		assert   func(t *testing.T, commands []helpers.RecordedCommand, depCacheDir, liveCacheDir string)
		name     string
		fixture  string
		flags    string
		depState dependencyOutputState
	}{
		{
			name:     "state read by default",
			fixture:  "nested-optimization-rustfs",
			depState: depTerraformDirRemoved,
			assert: func(t *testing.T, commands []helpers.RecordedCommand, _, liveCacheDir string) {
				t.Helper()

				assert.False(
					t,
					slices.ContainsFunc(commands, func(c helpers.RecordedCommand) bool {
						return (isOutputJSON(c) || isDependencyOutputInit(c)) && c.Dir != liveCacheDir
					}),
					"expected output reads only in %s, got %v",
					liveCacheDir,
					commands,
				)
			},
		},
		{
			name:     "bare init with generated backend",
			fixture:  "nested-optimization-rustfs",
			flags:    "--no-dependency-fetch-output-from-state",
			depState: depTerraformDirRemoved,
			assert: func(t *testing.T, commands []helpers.RecordedCommand, _, _ string) {
				t.Helper()

				assertDependencyOutputFromBackend(t, commands)
			},
		},
		{
			name:     "bare init with backend block",
			fixture:  "nested-optimization-nogen-rustfs",
			flags:    "--no-dependency-fetch-output-from-state",
			depState: depTerraformDirRemoved,
			assert: func(t *testing.T, commands []helpers.RecordedCommand, _, _ string) {
				t.Helper()

				assertDependencyOutputFromBackend(t, commands)
			},
		},
		{
			name:     "output from init-ed working dir",
			fixture:  "nested-optimization-rustfs",
			flags:    "--no-dependency-fetch-output-from-state",
			depState: depTerraformDirKept,
			assert: func(t *testing.T, commands []helpers.RecordedCommand, depCacheDir, _ string) {
				t.Helper()

				assert.False(
					t,
					slices.ContainsFunc(commands, isDependencyOutputInit),
					"unexpected bare init, got %v",
					commands,
				)
				assert.True(
					t,
					slices.ContainsFunc(commands, func(c helpers.RecordedCommand) bool {
						return isOutputJSON(c) && c.Dir == depCacheDir
					}),
					"expected output -json in %s, got %v",
					depCacheDir,
					commands,
				)
			},
		},
	}

	for _, tc := range testCases { //nolint:paralleltest // the parent sets RustFS credentials with t.Setenv, which bars t.Parallel
		t.Run(tc.name, func(t *testing.T) {
			s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
			rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, tc.fixture)
			rootTerragruntConfigPath := filepath.Join(rootPath, "root.hcl")
			livePath := filepath.Join(rootPath, "live")

			helpers.CopyAndFillMapPlaceholders(
				t,
				rootTerragruntConfigPath,
				rootTerragruntConfigPath,
				map[string]string{
					"__FILL_IN_BUCKET_NAME__": s3BucketName,
					"__FILL_IN_S3_ENDPOINT__": rustfsAddr,
				},
			)

			helpers.RunTerragrunt(
				t,
				"terragrunt run --all apply --non-interactive --backend-bootstrap --working-dir "+rootPath,
			)

			depCacheDir := helpers.FindCacheWorkingDir(t, filepath.Join(rootPath, "dep"))
			require.NotEmpty(t, depCacheDir, "Cache directory for dep should exist")

			if tc.depState == depTerraformDirRemoved {
				helpers.CleanupTerraformFolder(t, depCacheDir)
			}

			deepDepCacheDir := helpers.FindCacheWorkingDir(t, filepath.Join(rootPath, "deepdep"))
			require.NotEmpty(t, deepDepCacheDir, "Cache directory for deepdep should exist")
			require.NoError(t, os.Remove(filepath.Join(deepDepCacheDir, "terraform.tfstate")))

			recorder := helpers.NewExecRecorder(vexec.NewOSExec())
			stdout := bytes.Buffer{}
			v := venv.OSVenv().WithExec(recorder)
			v.Writers = &writer.Writers{Writer: &stdout, ErrWriter: os.Stderr}

			err := helpers.RunTerragruntCommandWithVenv(
				t,
				t.Context(),
				v,
				"terragrunt run "+tc.flags+" --non-interactive --working-dir "+livePath+" -- output -no-color -json",
			)
			require.NoError(t, err)

			outputs := map[string]helpers.TerraformOutput{}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
			assert.Equal(t, `They said, "No, The answer is 42"`, outputs["output"].Value)

			tc.assert(t, recorder.Commands(), depCacheDir, helpers.FindCacheWorkingDir(t, livePath))
		})
	}
}

// dependencyOutputState says whether a dependency output test removes the dep
// unit's .terraform directory before reading outputs.
type dependencyOutputState int

const (
	depTerraformDirKept dependencyOutputState = iota
	depTerraformDirRemoved
)

// isDependencyOutputInit reports whether c is the init Terragrunt runs before
// reading a dependency's outputs from its backend.
func isDependencyOutputInit(c helpers.RecordedCommand) bool {
	return len(c.Args) > 0 && c.Args[0] == tf.CommandNameInit && slices.Contains(c.Args, "-get=false")
}

// isOutputJSON reports whether c reads outputs as JSON.
func isOutputJSON(c helpers.RecordedCommand) bool {
	return len(c.Args) > 0 && c.Args[0] == tf.CommandNameOutput && slices.Contains(c.Args, "-json")
}

// assertDependencyOutputFromBackend asserts that dependency outputs were read
// by running output in the directory where Terragrunt ran a bare init against
// the dependency's backend.
func assertDependencyOutputFromBackend(t *testing.T, commands []helpers.RecordedCommand) {
	t.Helper()

	initIdx := slices.IndexFunc(commands, isDependencyOutputInit)
	require.NotEqual(t, -1, initIdx, "expected a bare init for the dep backend, got %v", commands)

	initDir := commands[initIdx].Dir

	assert.True(
		t,
		slices.ContainsFunc(commands[initIdx+1:], func(c helpers.RecordedCommand) bool {
			return isOutputJSON(c) && c.Dir == initDir
		}),
		"expected output -json in %s after the bare init, got %v",
		initDir,
		commands,
	)
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
