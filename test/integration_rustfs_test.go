//go:build docker

package test_test

import (
	"bytes"
	"encoding/json"
	"maps"
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

// TestRustFSOutputFromRemoteState pins how app2 reads the outputs of app1 and
// app3 once their local working dirs are gone: from the state objects by
// default, and through tofu with --no-dependency-fetch-output-from-state. It
// also pins that run --all output prints units in dependency order.
func TestRustFSOutputFromRemoteState(t *testing.T) {
	t.Parallel()

	rustfsAddr := setupRustFS(t)

	testCases := []struct {
		assert func(t *testing.T, commands []helpers.RecordedCommand)
		name   string
		flags  string
	}{
		{
			name: "state read by default",
			assert: func(t *testing.T, commands []helpers.RecordedCommand) {
				t.Helper()

				// Neither run asks tofu for JSON outputs, so any output -json or
				// bare init is a dependency read.
				assert.False(
					t,
					slices.ContainsFunc(commands, func(c helpers.RecordedCommand) bool {
						return isOutputJSON(c) || isDependencyOutputInit(c)
					}),
					"expected no dependency output reads through tofu, got %v",
					commands,
				)
			},
		},
		{
			name:  "tofu output when state read is disabled",
			flags: "--no-dependency-fetch-output-from-state",
			assert: func(t *testing.T, commands []helpers.RecordedCommand) {
				t.Helper()

				assertDependencyOutputFromBackend(t, commands)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			environmentPath := setupOutputFromRemoteStateRustFS(t, rustfsAddr)

			for _, app := range []string{"app1", "app3"} {
				_, _, err := runTerragruntRustFS(
					t,
					"terragrunt run --backend-bootstrap --non-interactive --working-dir "+
						filepath.Join(environmentPath, app)+" -- apply -auto-approve",
				)
				require.NoError(t, err)

				cacheDir := helpers.FindCacheWorkingDir(t, filepath.Join(environmentPath, app))
				require.NotEmpty(t, cacheDir, "Cache directory for %s should exist", app)
				require.NoError(t, os.RemoveAll(filepath.Join(cacheDir, ".terraform")))
			}

			recorder := helpers.NewExecRecorder(vexec.NewOSExec())
			run := func(command string) string {
				t.Helper()

				stdout := bytes.Buffer{}
				v := rustfsVenv().WithExec(recorder)
				v.Writers = &writer.Writers{Writer: &stdout, ErrWriter: os.Stderr}

				require.NoError(t, helpers.RunTerragruntCommandWithVenv(t, t.Context(), v, command))

				return stdout.String()
			}

			run(
				"terragrunt run --backend-bootstrap " + tc.flags + " --non-interactive --working-dir " +
					filepath.Join(environmentPath, "app2") + " -- apply -auto-approve",
			)

			stdout := run(
				"terragrunt run --all output --backend-bootstrap " + tc.flags +
					" --non-interactive --working-dir " + environmentPath,
			)

			app1Idx := strings.Index(stdout, "app1 output")
			app2Idx := strings.Index(stdout, "app2 output")
			app3Idx := strings.Index(stdout, "app3 output")

			require.NotEqual(t, -1, app1Idx, "stdout=%s", stdout)
			require.NotEqual(t, -1, app2Idx, "stdout=%s", stdout)
			require.NotEqual(t, -1, app3Idx, "stdout=%s", stdout)
			assert.Less(t, app3Idx, app1Idx, "app3 must print before app1, which depends on it")
			assert.Less(t, app1Idx, app2Idx, "app1 must print before app2, which depends on it")

			tc.assert(t, recorder.Commands())
		})
	}
}

// TestRustFSMockOutputsFromRemoteState pins that app2 falls back to its
// mock_outputs for a dependency with no state, both when the bucket exists but
// the state object does not, and when the bucket itself does not exist yet.
func TestRustFSMockOutputsFromRemoteState(t *testing.T) {
	t.Parallel()

	rustfsAddr := setupRustFS(t)

	const mockOutput = "(known after run --all apply)"

	testCases := []struct {
		name        string
		wantApp1    string
		wantApp3    string
		appliedDeps []string
	}{
		{
			name:        "missing state object",
			appliedDeps: []string{"app1"},
			wantApp1:    "app1 output",
			wantApp3:    mockOutput,
		},
		{
			name:     "missing bucket",
			wantApp1: mockOutput,
			wantApp3: mockOutput,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			environmentPath := setupOutputFromRemoteStateRustFS(t, rustfsAddr)

			for _, app := range tc.appliedDeps {
				_, _, err := runTerragruntRustFS(
					t,
					"terragrunt run --backend-bootstrap --non-interactive --working-dir "+
						filepath.Join(environmentPath, app)+" -- apply -auto-approve",
				)
				require.NoError(t, err)
			}

			app2Path := filepath.Join(environmentPath, "app2")

			// app2 passes its dependency outputs through as its own outputs, so
			// its state records the values the apply resolved.
			_, _, err := runTerragruntRustFS(
				t,
				"terragrunt run --backend-bootstrap --non-interactive --working-dir "+app2Path+" -- apply -auto-approve",
			)
			require.NoError(t, err)

			stdout, _, err := runTerragruntRustFS(
				t,
				"terragrunt run --non-interactive --working-dir "+app2Path+" -- output -no-color -json",
			)
			require.NoError(t, err)

			outputs := map[string]helpers.TerraformOutput{}
			require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))
			assert.Equal(t, tc.wantApp1, outputs["app1_text"].Value)
			assert.Equal(t, tc.wantApp3, outputs["app3_text"].Value)
		})
	}
}

// TestRustFSStackDependencyMockOutputs covers stack dependency mock resolution against a live S3
// API, where a unit that hasn't been applied yet has no state to read. It pins that a map-typed
// mock_outputs resolves for such a unit, both before anything is applied and once only part of
// the stack is, and that a unit is never dropped silently from the aggregated stack outputs:
// neither when mock_outputs_allowed_terraform_commands rules its mocks out for the current command,
// nor when mock_outputs can't be keyed by unit name at all.
func TestRustFSStackDependencyMockOutputs(t *testing.T) {
	t.Parallel()

	rustfsAddr := setupRustFS(t)

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

	_, _, err := runTerragruntRustFS(t, "terragrunt stack generate --working-dir "+rootPath)
	require.NoError(t, err)

	stackPath := filepath.Join(rootPath, ".terragrunt-stack")
	appPath := filepath.Join(stackPath, "app")
	vpcPath := filepath.Join(stackPath, "networking", ".terragrunt-stack", "vpc")

	planApp := func() string {
		t.Helper()

		stdout, stderr, err := runTerragruntRustFS(
			t,
			"terragrunt plan --dependency-fetch-output-from-state --backend-bootstrap --non-interactive --working-dir "+appPath,
		)
		require.NoError(
			t,
			err,
			"a map-typed mock_outputs must resolve for the stack unit with no state; stderr=%s",
			stderr,
		)

		return stdout
	}

	stdout := planApp()
	assert.Contains(t, stdout, "mock-vpc-id", "an unapplied unit must resolve to its mock")
	assert.Contains(t, stdout, "mock-subnet-id", "an unapplied unit must resolve to its mock")

	_, _, err = runTerragruntRustFS(
		t,
		"terragrunt apply --backend-bootstrap --auto-approve --non-interactive --working-dir "+vpcPath,
	)
	require.NoError(t, err)

	stdout = planApp()
	assert.Contains(t, stdout, "real-vpc-id", "the applied unit must resolve to its real output")
	assert.Contains(t, stdout, "mock-subnet-id", "the unapplied unit must resolve to its mock")

	strictPath := filepath.Join(stackPath, "strict")

	_, _, err = runTerragruntRustFS(
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

	_, stderr, err := runTerragruntRustFS(
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

	_, stderr, err = runTerragruntRustFS(
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
// deepdep's local state and checks which commands the live unit spawns, or
// that the read fails when dependency optimization is disabled.
func TestRustFSDependencyOutputOptimization(t *testing.T) {
	t.Parallel()

	rustfsAddr := setupRustFS(t)

	testCases := []struct {
		assert   func(t *testing.T, commands []helpers.RecordedCommand, depCacheDir, liveCacheDir string)
		errAs    any
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
		{
			// With the optimization disabled, reading dep's outputs resolves
			// dep's own inputs, which need deepdep's missing local state.
			name:     "optimization disabled",
			fixture:  "nested-optimization-disable-rustfs",
			depState: depTerraformDirKept,
			errAs:    new(config.TerragruntOutputTargetNoOutputs),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

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

			_, _, err := runTerragruntRustFS(
				t,
				"terragrunt run --all apply --non-interactive --backend-bootstrap --working-dir "+rootPath,
			)
			require.NoError(t, err)

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
			v := rustfsVenv().WithExec(recorder)
			v.Writers = &writer.Writers{Writer: &stdout, ErrWriter: os.Stderr}

			err = helpers.RunTerragruntCommandWithVenv(
				t,
				t.Context(),
				v,
				"terragrunt run "+tc.flags+" --non-interactive --working-dir "+livePath+" -- output -no-color -json",
			)

			if tc.errAs != nil {
				require.ErrorAs(t, err, tc.errAs)

				return
			}

			require.NoError(t, err)

			outputs := map[string]helpers.TerraformOutput{}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
			assert.Equal(t, `They said, "No, The answer is 42"`, outputs["output"].Value)

			tc.assert(t, recorder.Commands(), depCacheDir, helpers.FindCacheWorkingDir(t, livePath))
		})
	}
}

// setupOutputFromRemoteStateRustFS copies the output-from-remote-state-rustfs
// fixture, points it at a new bucket on rustfsAddr, and returns the path of
// its env1 directory. The bucket is not created.
func setupOutputFromRemoteStateRustFS(t *testing.T, rustfsAddr string) string {
	t.Helper()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputFromRemoteStateRustFS)
	fixturePath := filepath.Join(tmpEnvPath, testFixtureOutputFromRemoteStateRustFS)
	rootTerragruntConfigPath := filepath.Join(fixturePath, "root.hcl")

	helpers.CopyAndFillMapPlaceholders(
		t,
		rootTerragruntConfigPath,
		rootTerragruntConfigPath,
		map[string]string{
			"__FILL_IN_BUCKET_NAME__": "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID()),
			"__FILL_IN_S3_ENDPOINT__": rustfsAddr,
		},
	)

	return filepath.Join(fixturePath, "env1")
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

// runTerragruntRustFS runs command with [rustfsVenv] and returns its stdout and
// stderr.
func runTerragruntRustFS(t *testing.T, command string) (string, string, error) {
	t.Helper()

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	v := rustfsVenv()
	v.Writers = &writer.Writers{Writer: &stdout, ErrWriter: &stderr}

	err := helpers.RunTerragruntCommandWithVenv(t, t.Context(), v, command)

	helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "stderr")

	return stdout.String(), stderr.String(), err
}

// rustfsVenv returns an OS venv whose environment carries the RustFS
// credentials in place of any AWS_* variables from the process. Terragrunt
// reads AWS credentials from the venv and hands tofu the venv's environment, so
// tests that use it can run in parallel without setting process environment
// variables.
//
// The AWS SDK still reads AWS_PROFILE from the process environment, so a run
// fails when it names a profile the machine does not have.
func rustfsVenv() *venv.Venv {
	v := venv.OSVenv()

	maps.DeleteFunc(v.Env, func(k, _ string) bool {
		return strings.HasPrefix(k, "AWS_")
	})

	v.Env["AWS_ACCESS_KEY_ID"] = "rustfsadmin"
	v.Env["AWS_SECRET_ACCESS_KEY"] = "rustfsadmin"
	v.Env["AWS_DEFAULT_REGION"] = "us-east-1"

	return v
}

// setupRustFS starts a RustFS container and returns its endpoint URL.
//
// Fails the test when the process sets AWS_PROFILE, because the AWS SDK reads
// it from the process environment even when the venv omits it.
func setupRustFS(t *testing.T) string {
	t.Helper()

	require.Empty(
		t,
		venv.OSVenv().Env["AWS_PROFILE"],
		"unset AWS_PROFILE to run RustFS tests: the AWS SDK reads it from the process environment, not the venv",
	)

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
