package test_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
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

			helpers.CleanupTerraformFolder(t, testFixtureExecCmd)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExecCmd)

			rootPath := util.JoinPath(tmpEnvPath, testFixtureExecCmd, "app")
			rootPath, err := filepath.EvalSymlinks(rootPath)
			require.NoError(t, err)

			downloadDirPath := util.JoinPath(rootPath, ".terragrunt-cache")
			scriptPath := util.JoinPath(tmpEnvPath, testFixtureExecCmd, tc.scriptPath)

			err = os.Mkdir(downloadDirPath, os.ModePerm)
			require.NoError(t, err)

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt exec --working-dir "+rootPath+" "+strings.Join(tc.args, " ")+" -- "+scriptPath)
			require.NoError(t, err)
			assert.Contains(t, stdout, "The first arg is arg1. The second arg is arg2. The script is running in the directory "+util.JoinPath(rootPath, tc.runInDir))
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

			helpers.CleanupTerraformFolder(t, testFixtureExecCmdTfPath)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExecCmdTfPath)

			rootPath := util.JoinPath(tmpEnvPath, testFixtureExecCmdTfPath, "app")
			rootPath, err := filepath.EvalSymlinks(rootPath)
			require.NoError(t, err)

			downloadDirPath := util.JoinPath(rootPath, ".terragrunt-cache")
			scriptPath := util.JoinPath(tmpEnvPath, testFixtureExecCmdTfPath, "./script.sh")

			tfPath := ""
			if tc.tfPath != "" {
				tfPath = "--tf-path " + util.JoinPath(tmpEnvPath, testFixtureExecCmdTfPath, tc.tfPath)
			}

			err = os.Mkdir(downloadDirPath, os.ModePerm)
			require.NoError(t, err)

			depPath := util.JoinPath(tmpEnvPath, testFixtureExecCmdTfPath, "dep")
			depStdout := bytes.Buffer{}
			depStderr := bytes.Buffer{}
			require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive -no-color --no-color --log-format=pretty --working-dir "+depPath, &depStdout, &depStderr))

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt --log-level debug exec "+tfPath+" --working-dir "+rootPath+"  -- "+scriptPath)
			require.NoError(t, err)
			assert.Contains(t, stdout, tc.expected)
		})
	}
}
