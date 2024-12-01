package test_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

const (
	testSimpleErrors = "fixtures/errors/default"
	testIgnoreErrors = "fixtures/errors/ignore"
)

func TestErrorsHandling(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testSimpleErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testSimpleErrors)
	rootPath := util.JoinPath(tmpEnvPath, testSimpleErrors)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	require.NoError(t, err)
}

func TestIgnoreError(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testIgnoreErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testIgnoreErrors)
	rootPath := util.JoinPath(tmpEnvPath, testIgnoreErrors)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Ignoring error example1")

}
