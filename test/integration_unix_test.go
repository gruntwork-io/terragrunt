//go:build linux || darwin
// +build linux darwin

package integration_test

import (
	"testing"
)

const (
	TEST_FIXTURE_LOCAL_RELATIVE_ARGS_UNIX_DOWNLOAD_PATH = "fixture-download/local-relative-extra-args-unix"
)

func TestLocalWithRelativeExtraArgsUnix(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_LOCAL_RELATIVE_ARGS_UNIX_DOWNLOAD_PATH)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+TEST_FIXTURE_LOCAL_RELATIVE_ARGS_UNIX_DOWNLOAD_PATH)

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+TEST_FIXTURE_LOCAL_RELATIVE_ARGS_UNIX_DOWNLOAD_PATH)
}
