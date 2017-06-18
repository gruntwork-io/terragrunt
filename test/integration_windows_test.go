// +build windows

package test

import (
	"fmt"
	"testing"
)

const (
	TEST_FIXTURE_LOCAL_RELATIVE_ARGS_WINDOWS_DOWNLOAD_PATH = "fixture-download/local-relative-extra-args-windows"
)

func TestLocalWithRelativeExtraArgsWindows(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_RELATIVE_ARGS_WINDOWS_DOWNLOAD_PATH)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_RELATIVE_ARGS_WINDOWS_DOWNLOAD_PATH))

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, fmt.Sprintf("terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir %s", TEST_FIXTURE_LOCAL_RELATIVE_ARGS_WINDOWS_DOWNLOAD_PATH))
}
