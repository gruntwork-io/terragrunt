//go:build tf

package test_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFExecCommand(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since bash script execution is not supported")
	}

	testCases := []struct {
		scriptPath string
		runInDir   string
		args       []string
	}{
		{
			scriptPath: "./script.sh arg1 arg2",
			runInDir:   "",
		},
		{
			args:       []string{"--in-download-dir"},
			scriptPath: "./script.sh arg1 arg2",
			runInDir:   ".terragrunt-cache",
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureExecCmd)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExecCmd)

			rootPath := filepath.Join(tmpEnvPath, testFixtureExecCmd, "app")
			rootPath, err := filepath.EvalSymlinks(rootPath)
			require.NoError(t, err)

			downloadDirPath := filepath.Join(rootPath, ".terragrunt-cache")
			scriptPath := filepath.Join(tmpEnvPath, testFixtureExecCmd, tc.scriptPath)

			err = os.Mkdir(downloadDirPath, os.ModePerm)
			require.NoError(t, err)

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt exec --working-dir "+rootPath+" "+strings.Join(
					tc.args,
					" ",
				)+" -- "+scriptPath,
			)
			require.NoError(t, err)
			assert.Contains(
				t,
				stdout,
				"The first arg is arg1. The second arg is arg2. The script is running in the directory "+filepath.Join(
					rootPath,
					tc.runInDir,
				),
			)
		})
	}
}

func TestTFExecCommandTfPath(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since bash script execution is not supported")
	}

	testCases := []struct {
		expected string
		tfPath   string
	}{
		{
			expected: "baz is baz",
		},
		{
			expected: "baz is terraform",
			tfPath:   "terraform-output-json.sh",
		},
		{
			expected: "baz is tofu",
			tfPath:   "tofu-output-json.sh",
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureExecCmdTfPath)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExecCmdTfPath)

			rootPath := filepath.Join(tmpEnvPath, testFixtureExecCmdTfPath, "app")
			rootPath, err := filepath.EvalSymlinks(rootPath)
			require.NoError(t, err)

			downloadDirPath := filepath.Join(rootPath, ".terragrunt-cache")
			scriptPath := filepath.Join(tmpEnvPath, testFixtureExecCmdTfPath, "./script.sh")

			tfPath := ""
			if tc.tfPath != "" {
				tfPath = "--tf-path " + filepath.Join(
					tmpEnvPath,
					testFixtureExecCmdTfPath,
					tc.tfPath,
				)
			}

			err = os.Mkdir(downloadDirPath, os.ModePerm)
			require.NoError(t, err)

			depPath := filepath.Join(tmpEnvPath, testFixtureExecCmdTfPath, "dep")
			depStdout := bytes.Buffer{}
			depStderr := bytes.Buffer{}
			require.NoError(
				t,
				helpers.RunTerragruntCommand(
					t,
					"terragrunt apply -auto-approve --non-interactive -no-color --no-color --log-format=pretty --working-dir "+depPath,
					&depStdout,
					&depStderr,
				),
			)

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt --log-level debug exec "+tfPath+" --working-dir "+rootPath+"  -- "+scriptPath,
			)
			require.NoError(t, err)
			assert.Contains(t, stdout, tc.expected)
		})
	}
}

// TestTFExecCommandSourceMap proves that `exec` honors `--source-map`: the unit's source points at a
// repository that does not exist, so the command only reaches the module when the map redirects it.
func TestTFExecCommandSourceMap(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since `cat` is not available")
	}

	helpers.CleanupTerraformFolder(t, testFixtureExecCmdSourceMap)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExecCmdSourceMap)

	rootPath := filepath.Join(tmpEnvPath, testFixtureExecCmdSourceMap, "unit")

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt exec --non-interactive --working-dir "+rootPath+
			" --source-map git::ssh://git@github.com/gruntwork-io/i-dont-exist.git="+tmpEnvPath+
			" --in-download-dir -- cat main.tf",
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "mapped_module_id")
}

// TestTFExecCommandNoAutoInit proves that `exec --in-download-dir` honors `--no-auto-init`: the
// download directory is freshly populated, so init is needed, and the flag turns it into a warning.
func TestTFExecCommandNoAutoInit(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since `cat` is not available")
	}

	helpers.CleanupTerraformFolder(t, testFixtureExecCmdSourceMap)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExecCmdSourceMap)

	rootPath := filepath.Join(tmpEnvPath, testFixtureExecCmdSourceMap, "unit")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt exec --non-interactive --working-dir "+rootPath+
			" --source-map git::ssh://git@github.com/gruntwork-io/i-dont-exist.git="+tmpEnvPath+
			" --in-download-dir --no-auto-init -- cat main.tf",
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "mapped_module_id")
	assert.Contains(t, stderr, "Detected that init is needed, but Auto-Init is disabled")
	assert.NotContains(t, stderr, "has been successfully initialized")
}

// TestTFExecCommandNoAutoInitDependency proves that `--no-auto-init` reaches dependency units even
// without `--in-download-dir`, since `exec` initializes those while resolving their outputs.
func TestTFExecCommandNoAutoInitDependency(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		args           string
		expectedStderr string
	}{
		{
			name:           "auto-init initializes the dependency",
			expectedStderr: "Initializing the backend",
		},
		{
			name:           "no-auto-init warns instead",
			args:           " --no-auto-init",
			expectedStderr: "Detected that init is needed, but Auto-Init is disabled",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExecCmdDependency)
			rootPath := filepath.Join(tmpEnvPath, testFixtureExecCmdDependency, "app")

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt exec --non-interactive --working-dir "+rootPath+tc.args+" -- env",
			)
			require.NoError(t, err)
			assert.Contains(t, stdout, "TF_VAR_id=mock")
			assert.Contains(t, stderr, tc.expectedStderr)
		})
	}
}
