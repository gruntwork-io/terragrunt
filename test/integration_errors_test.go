package test_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

const (
	testSimpleErrors = "fixtures/errors/default"
)

func TestErrorsHandling(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testSimpleErrors)
	tmpEnvPath := helpers.CopyEnvironment(t, testSimpleErrors)
	rootPath := util.JoinPath(tmpEnvPath, testSimpleErrors)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	require.NoError(t, err)
}
