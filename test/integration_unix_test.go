//go:build linux || darwin
// +build linux darwin

package test_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureDownloadPath                      = "fixtures/download"
	testFixtureLocalRelativeArgsUnixDownloadPath = "fixtures/download/local-relative-extra-args-unix"
)

func TestLocalWithRelativeExtraArgsUnix(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDownloadPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureLocalRelativeArgsUnixDownloadPath)

	testPath, err := filepath.EvalSymlinks(testPath)
	require.NoError(t, err)

	helpers.CleanupTerraformFolder(t, testPath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testPath)
}
