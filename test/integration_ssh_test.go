//go:build ssh

// We don't want contributors to have to install SSH keys to run these tests, so we skip
// them by default. Contributors need to opt in to run these tests by setting the
// build flag `ssh` when running the tests. This is done by adding the `-tags ssh` flag
// to the `go test` command. For example:
//
// go test -tags ssh ./...

package test_test

import (
	"bytes"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHSourceMapWithSlashInRef(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSourceMapSlashes)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureSourceMapSlashes)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --source-map git::ssh://git@github.com/gruntwork-io/i-dont-exist.git=git::git@github.com:gruntwork-io/terragrunt.git?ref=fixture/test-fixtures --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
}

func TestSSHTerragruntNoWarningRemotePath(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoSubmodules)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureNoSubmodules)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt init --non-interactive --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	assert.NotContains(t, stderr.String(), "No double-slash (//) found in source URL")
}

func TestSSHDownloadSourceWithRef(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRefSource)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureRefSource)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
}
