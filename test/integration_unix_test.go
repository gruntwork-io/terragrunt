//go:build linux || darwin
// +build linux darwin

package test_test

import (
	"testing"
)

const (
	TestFixtureLocalRelativeArgsUnixDownloadPath = "fixtures/download/local-relative-extra-args-unix"
)

func TestLocalWithRelativeExtraArgsUnix(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TestFixtureLocalRelativeArgsUnixDownloadPath)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+TestFixtureLocalRelativeArgsUnixDownloadPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+TestFixtureLocalRelativeArgsUnixDownloadPath)
}
