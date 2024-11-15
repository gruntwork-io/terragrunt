package test_test

import (
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"testing"
)

const (
	testExcludeByDefault = "fixtures/exclude/exclude-default"
)

func TestExcludeByDefault(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testExcludeByDefault)
	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeByDefault)
	rootPath := util.JoinPath(tmpEnvPath, testExcludeByDefault)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	require.NoError(t, err)

	assert.Contains(t, stderr, "app1")
	assert.NotContains(t, stderr, "app2")
}
