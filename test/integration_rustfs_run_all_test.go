//go:build docker

package test_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureOutputAllRustFS              = "fixtures/output-all-rustfs"
	testFixtureOutputFromDependencyRustFS   = "fixtures/output-from-dependency-rustfs"
	testFixtureGetOutputRegression906RustFS = "regression-906-rustfs"
)

// TestRustFSRunAll pins run --all against an S3 backend on a three-unit stack
// where app1 depends on app3 and app2 depends on both.
func TestRustFSRunAll(t *testing.T) {
	t.Parallel()

	rustfsAddr := setupRustFS(t)

	t.Run("init runs in every unit", func(t *testing.T) {
		t.Parallel()

		envPath := copyOutputAllRustFS(t, rustfsAddr)
		recorder := helpers.NewExecRecorder(vexec.NewOSExec())

		err := helpers.RunTerragruntCommandWithVenv(
			t,
			t.Context(),
			rustfsVenv().WithExec(recorder),
			"terragrunt run --all init --backend-bootstrap --non-interactive --working-dir "+envPath,
		)
		require.NoError(t, err)

		assertRanInEveryOutputAllUnit(t, recorder.Commands(), tf.CommandNameInit, envPath)
	})

	t.Run("validate runs in every unit", func(t *testing.T) {
		t.Parallel()

		envPath := copyOutputAllRustFS(t, rustfsAddr)
		recorder := helpers.NewExecRecorder(vexec.NewOSExec())

		err := helpers.RunTerragruntCommandWithVenv(
			t,
			t.Context(),
			rustfsVenv().WithExec(recorder),
			"terragrunt run --all validate --backend-bootstrap --non-interactive --working-dir "+envPath,
		)
		require.NoError(t, err)

		assertRanInEveryOutputAllUnit(t, recorder.Commands(), tf.CommandNameValidate, envPath)
	})

	t.Run("output follows dependency order", func(t *testing.T) {
		t.Parallel()

		envPath := copyOutputAllRustFS(t, rustfsAddr)

		_, _, err := runTerragruntRustFS(
			t,
			"terragrunt run --all apply --non-interactive --backend-bootstrap --working-dir "+envPath,
		)
		require.NoError(t, err)

		stdout, _, err := runTerragruntRustFS(
			t,
			"terragrunt run --all output --non-interactive --backend-bootstrap --working-dir "+envPath,
		)
		require.NoError(t, err)

		app3Idx := strings.Index(stdout, "app3 output")
		app1Idx := strings.Index(stdout, "app1 output")
		app2Idx := strings.Index(stdout, "app2 output")

		require.NotEqual(t, -1, app3Idx, "missing app3 output in %q", stdout)
		require.NotEqual(t, -1, app1Idx, "missing app1 output in %q", stdout)
		require.NotEqual(t, -1, app2Idx, "missing app2 output in %q", stdout)
		assert.Less(t, app3Idx, app1Idx, "app3 output must precede app1 output")
		assert.Less(t, app1Idx, app2Idx, "app1 output must precede app2 output")

		// app1 and app3 have no app2_text output, so without --queue-ignore-errors
		// app2 never runs.
		stdout, _, err = runTerragruntRustFS(
			t,
			"terragrunt run --all output app2_text --queue-ignore-errors --non-interactive "+
				"--backend-bootstrap --working-dir "+envPath,
		)
		require.Error(t, err)
		assert.Contains(t, stdout, "app2 output")
	})

	// stdin holds a single answer, so a second prompt reads EOF and fails the
	// run. A prompt that is never shown leaves the answer unread.
	t.Run("missing bucket prompts once", func(t *testing.T) {
		t.Parallel()

		envPath := copyOutputAllRustFS(t, rustfsAddr)
		stdin := stdinPipe(t, "y\n")

		err := helpers.RunTerragruntCommandWithVenv(
			t,
			t.Context(),
			rustfsVenv().WithStdin(stdin),
			"terragrunt run --backend-bootstrap --all init --working-dir "+envPath,
		)
		require.NoError(t, err)

		unread, err := io.ReadAll(stdin)
		require.NoError(t, err)
		assert.Empty(t, unread, "the bucket prompt must consume the answer")
	})

	t.Run("apply asks for confirmation", func(t *testing.T) {
		t.Parallel()

		envPath := copyOutputAllRustFS(t, rustfsAddr)
		recorder := helpers.NewExecRecorder(vexec.NewOSExec())

		err := helpers.RunTerragruntCommandWithVenv(
			t,
			t.Context(),
			rustfsVenv().WithExec(recorder).WithStdin(stdinPipe(t, "n\n")),
			"terragrunt run --all apply --working-dir "+envPath,
		)
		require.ErrorIs(t, err, runall.ErrUserCancelled)
		assert.False(
			t,
			slices.ContainsFunc(recorder.Commands(), func(c helpers.RecordedCommand) bool {
				return len(c.Args) > 0 && c.Args[0] == tf.CommandNameApply
			}),
			"no unit may apply after the prompt is declined, got %v",
			recorder.Commands(),
		)
	})
}

// TestRustFSRunAllDependencyOutputs pins reading dependency outputs from an S3
// backend during run --all apply.
func TestRustFSRunAllDependencyOutputs(t *testing.T) {
	t.Parallel()

	rustfsAddr := setupRustFS(t)

	// AWS_CSM_ENABLED once made dependency output reads fail to parse.
	t.Run("AWS_CSM_ENABLED", func(t *testing.T) {
		t.Parallel()

		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputFromDependencyRustFS)
		rootPath := filepath.Join(tmpEnvPath, testFixtureOutputFromDependencyRustFS)
		depConfigPath := filepath.Join(rootPath, "dependency", "terragrunt.hcl")

		helpers.CopyAndFillMapPlaceholders(
			t,
			depConfigPath,
			depConfigPath,
			map[string]string{
				"__FILL_IN_BUCKET_NAME__": "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID()),
				"__FILL_IN_S3_ENDPOINT__": rustfsAddr,
			},
		)

		v := rustfsVenv()
		v.Env["AWS_CSM_ENABLED"] = "true"
		v.Writers = &writer.Writers{Writer: os.Stderr, ErrWriter: os.Stderr}

		err := helpers.RunTerragruntCommandWithVenv(
			t,
			t.Context(),
			v,
			"terragrunt run --all --backend-bootstrap --non-interactive --working-dir "+rootPath+
				" --log-level trace -- apply -auto-approve",
		)
		require.NoError(t, err)

		stdout := bytes.Buffer{}
		v.Writers = &writer.Writers{Writer: &stdout, ErrWriter: os.Stderr}

		err = helpers.RunTerragruntCommandWithVenv(
			t,
			t.Context(),
			v,
			"terragrunt output -no-color -json --non-interactive --working-dir "+filepath.Join(rootPath, "app"),
		)
		require.NoError(t, err)

		outputs := map[string]helpers.TerraformOutput{}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
		assert.Equal(t, map[string]any{"foo": "test-foo-value"}, outputs["vpc_config"].Value)
	})

	// Regression test for https://github.com/gruntwork-io/terragrunt/issues/906,
	// where units reading the same dependency's outputs concurrently failed. The
	// failure is nondeterministic, so the run repeats.
	t.Run("same output read concurrently", func(t *testing.T) {
		t.Parallel()

		mirror := helpers.NewGitServer(t)

		for range 3 {
			tmpEnvPath := mirror.RenderFixture(testFixtureGetOutput)
			rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, testFixtureGetOutputRegression906RustFS)
			commonDepConfigPath := filepath.Join(rootPath, "common-dep", "terragrunt.hcl")

			helpers.CopyAndFillMapPlaceholders(
				t,
				commonDepConfigPath,
				commonDepConfigPath,
				map[string]string{
					"__FILL_IN_BUCKET_NAME__": "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID()),
					"__FILL_IN_S3_ENDPOINT__": rustfsAddr,
				},
			)

			_, _, err := runTerragruntRustFS(
				t,
				"terragrunt run --all apply --backend-bootstrap --source-update --non-interactive --working-dir "+rootPath,
			)
			require.NoError(t, err)
		}
	})
}

// stdinPipe returns the read end of a closed pipe holding input. It is a file
// rather than an in-memory reader so that subprocesses inherit it without
// draining it.
func stdinPipe(t *testing.T, input string) *os.File {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, r.Close())
	})

	_, err = w.WriteString(input)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	return r
}

// copyOutputAllRustFS copies the output-all fixture with a fresh bucket on the
// RustFS instance at rustfsAddr and returns the path to its env1 stack.
func copyOutputAllRustFS(t *testing.T, rustfsAddr string) string {
	t.Helper()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputAllRustFS)
	rootPath := filepath.Join(tmpEnvPath, testFixtureOutputAllRustFS)
	rootConfigPath := filepath.Join(rootPath, "root.hcl")

	helpers.CopyAndFillMapPlaceholders(
		t,
		rootConfigPath,
		rootConfigPath,
		map[string]string{
			"__FILL_IN_BUCKET_NAME__": "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID()),
			"__FILL_IN_S3_ENDPOINT__": rustfsAddr,
		},
	)

	return filepath.Join(rootPath, "env1")
}

// assertRanInEveryOutputAllUnit asserts that command ran in every unit of the
// output-all stack at envPath.
func assertRanInEveryOutputAllUnit(t *testing.T, commands []helpers.RecordedCommand, command, envPath string) {
	t.Helper()

	for _, unit := range []string{"app1", "app2", "app3"} {
		unitPath := filepath.Join(envPath, unit)

		assert.True(
			t,
			slices.ContainsFunc(commands, func(c helpers.RecordedCommand) bool {
				return len(c.Args) > 0 && c.Args[0] == command && (c.Dir == unitPath || strings.HasPrefix(c.Dir, unitPath+string(filepath.Separator)))
			}),
			"expected %s in %s, got %v",
			command,
			unitPath,
			commands,
		)
	}
}
