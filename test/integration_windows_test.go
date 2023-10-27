//go:build windows
// +build windows

package test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	TEST_FIXTURE_LOCAL_RELATIVE_ARGS_WINDOWS_DOWNLOAD_PATH = "fixture-download/local-relative-extra-args-windows"
)

func TestWindowsLocalWithRelativeExtraArgsWindows(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_RELATIVE_ARGS_WINDOWS_DOWNLOAD_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_RELATIVE_ARGS_WINDOWS_DOWNLOAD_PATH))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_RELATIVE_ARGS_WINDOWS_DOWNLOAD_PATH))
}

// TestWindowsTerragruntSourceMapDebug copies the test/fixture-source-map directory to a new Windows path
// and then ensures that the TERRAGRUNT_SOURCE_MAP env var can be used to swap out git sources for local modules
func TestWindowsTerragruntSourceMapDebug(t *testing.T) {
	testCases := []struct {
		name string
	}{
		{
			name: "multiple-match",
		},
		{
			name: "multiple-with-dependency",
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fixtureSourceMapPath := "fixture-source-map"
			cleanupTerraformFolder(t, fixtureSourceMapPath)
			targetPath := "C:\\test\\infrastructure-modules/"
			copyEnvironmentToPath(t, fixtureSourceMapPath, targetPath)
			rootPath := filepath.Join(targetPath, fixtureSourceMapPath)

			t.Setenv(
				"TERRAGRUNT_SOURCE_MAP",
				strings.Join(
					[]string{
						fmt.Sprintf("git::ssh://git@github.com/gruntwork-io/i-dont-exist.git=%s", targetPath),
						fmt.Sprintf("git::ssh://git@github.com/gruntwork-io/another-dont-exist.git=%s", targetPath),
					},
					",",
				),
			)
			tgPath := filepath.Join(rootPath, testCase.name)
			tgArgs := fmt.Sprintf("terragrunt run-all apply -auto-approve --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir %s", tgPath)
			runTerragrunt(t, tgArgs)
		})
	}
}

func TestWindowsTflintIsInvoked(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, TEST_FIXTURE_TFLINT_NO_ISSUES_FOUND)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_TFLINT_NO_ISSUES_FOUND)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir %s", modulePath), out, errOut)
	assert.NoError(t, err)

	assert.NotContains(t, errOut.String(), "Error while running tflint with args:")
	assert.NotContains(t, errOut.String(), "Tflint found issues in the project. Check for the tflint logs above.")

	found, err := regexp.MatchString(fmt.Sprintf("--config %s/.terragrunt-cache/.*/.tflint.hcl", modulePath), errOut.String())
	assert.NoError(t, err)
	assert.True(t, found)
}

func copyEnvironmentToPath(t *testing.T, environmentPath, targetPath string) {
	if err := os.MkdirAll(targetPath, 0777); err != nil {
		t.Fatalf("Failed to create temp dir %s due to error %v", targetPath, err)
	}

	copyErr := util.CopyFolderContents(environmentPath, util.JoinPath(targetPath, environmentPath), ".terragrunt-test", nil)
	require.NoError(t, copyErr)
}

func copyEnvironmentWithTflint(t *testing.T, environmentPath string) string {
	tmpDir, err := os.MkdirTemp("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}

	t.Logf("Copying %s to %s", environmentPath, tmpDir)

	require.NoError(t, util.CopyFolderContents(environmentPath, util.JoinPath(tmpDir, environmentPath), ".terragrunt-test", []string{".tflint.hcl"}))

	return tmpDir
}
