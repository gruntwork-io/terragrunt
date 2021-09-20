// +build windows

package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

			os.Setenv(
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
