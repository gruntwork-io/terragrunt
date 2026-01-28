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

func TestExecCommand(t *testing.T) {
	t.Parallel()

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

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExecCmd)

			rootPath := filepath.Join(tmpEnvPath, testFixtureExecCmd, "app")
			rootPath, err := filepath.EvalSymlinks(rootPath)
			require.NoError(t, err)

			downloadDirPath := filepath.Join(rootPath, ".terragrunt-cache")
			scriptPath := filepath.Join(tmpEnvPath, testFixtureExecCmd, tc.scriptPath)

			err = os.Mkdir(downloadDirPath, os.ModePerm)
			require.NoError(t, err)

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt exec --working-dir "+rootPath+" "+strings.Join(tc.args, " ")+" -- "+scriptPath)
			require.NoError(t, err)
			assert.Contains(t, stdout, "The first arg is arg1. The second arg is arg2. The script is running in the directory "+filepath.Join(rootPath, tc.runInDir))
		})
	}
}

func TestExecCommandTfPath(t *testing.T) {
	t.Parallel()

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

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExecCmdTfPath)

			rootPath := filepath.Join(tmpEnvPath, testFixtureExecCmdTfPath, "app")
			rootPath, err := filepath.EvalSymlinks(rootPath)
			require.NoError(t, err)

			downloadDirPath := filepath.Join(rootPath, ".terragrunt-cache")
			scriptPath := filepath.Join(tmpEnvPath, testFixtureExecCmdTfPath, "./script.sh")

			tfPath := ""
			if tc.tfPath != "" {
				tfPath = "--tf-path " + filepath.Join(tmpEnvPath, testFixtureExecCmdTfPath, tc.tfPath)
			}

			err = os.Mkdir(downloadDirPath, os.ModePerm)
			require.NoError(t, err)

			depPath := filepath.Join(tmpEnvPath, testFixtureExecCmdTfPath, "dep")
			depStdout := bytes.Buffer{}
			depStderr := bytes.Buffer{}
			require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive -no-color --no-color --log-format=pretty --working-dir "+depPath, &depStdout, &depStderr))

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt --log-level debug exec "+tfPath+" --working-dir "+rootPath+"  -- "+scriptPath)
			require.NoError(t, err)
			assert.Contains(t, stdout, tc.expected)
		})
	}
}
