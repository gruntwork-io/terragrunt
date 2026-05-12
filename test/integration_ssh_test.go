//nolint:paralleltest,tparallel // Every test in this file calls RequireSSH, which uses t.Setenv and therefore can't run in parallel.
package test_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHSourceMapWithSlashInRef(t *testing.T) {
	mirror := helpers.StartTerragruntMirror(t)
	mirror.RequireSSH(t)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSourceMapSlashes)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureSourceMapSlashes)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	// Source-map redirects the fake SSH URL in the fixture to the
	// local mirror. The `?ref=fixture/test-fixtures` is a slash-in-ref
	// regression check; the mirror exposes that branch so the
	// URL parser sees a real slash-bearing ref value.
	cmd := "terragrunt plan --non-interactive " +
		"--source-map git::ssh://git@github.com/gruntwork-io/i-dont-exist.git=git::" + mirror.SSHURL + "?ref=fixture/test-fixtures " +
		"--working-dir " + testPath
	require.NoError(t, helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr))
}

func TestSSHTerragruntNoWarningRemotePath(t *testing.T) {
	mirror := helpers.StartTerragruntMirror(t)
	mirror.RequireSSH(t)

	tmpEnvPath := mirror.RenderFixture(t, testFixtureNoSubmodules)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureNoSubmodules)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt init --non-interactive --working-dir "+testPath, &stdout, &stderr))
	assert.NotContains(t, stderr.String(), "No double-slash (//) found in source URL")
}

func TestSSHDownloadSourceWithRef(t *testing.T) {
	mirror := helpers.StartTerragruntMirror(t)
	mirror.RequireSSH(t)

	tmpEnvPath := mirror.RenderFixture(t, testFixtureRefSource)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureRefSource)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --working-dir "+testPath, &stdout, &stderr))
}
