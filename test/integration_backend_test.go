package test_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/require"
)

const testFixtureBackendUnappliedDependency = "fixtures/backend-unapplied-dependency"

func TestBackendBootstrapWithUnappliedDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureBackendUnappliedDependency)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureBackendUnappliedDependency)
	unitPath := filepath.Join(tmpEnvPath, testFixtureBackendUnappliedDependency, "foo")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt backend bootstrap --non-interactive --working-dir "+unitPath,
	)
	require.NoError(t, err)
}
