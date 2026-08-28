package test_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const (
	testFixtureDependencyExpansionKeyed = "fixtures/dependency-expansion/keyed"
	testFixtureDependencyExpansionMocks = "fixtures/dependency-expansion/mocks"
)

func TestDependencyExpansionReportsEveryInstanceAsDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureDependencyExpansionKeyed)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt find --no-color --dependencies --json --experiment block-iteration --working-dir "+
			testFixtureDependencyExpansionKeyed,
	)
	require.NoError(t, err)

	assert.Empty(t, stderr)

	requireJSONEqualIgnoringArrayOrder(t, `[
  {"type":"unit","path":"app","dependencies":["aurora-api","aurora-web","shard-0","shard-1","vpc"]},
  {"type":"unit","path":"aurora-api"},
  {"type":"unit","path":"aurora-web"},
  {"type":"unit","path":"shard-0"},
  {"type":"unit","path":"shard-1"},
  {"type":"unit","path":"vpc"}
]`, stdout)
}
