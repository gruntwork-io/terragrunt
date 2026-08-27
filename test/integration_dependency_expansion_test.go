package test_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

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

// requireJSONEqualIgnoringArrayOrder compares two JSON strings for equivalence, ignoring the order of array elements.
// Use it instead of require.JSONEq only when the output's array ordering is not guaranteed (e.g. the order of a unit's
// dependencies, or of units that share a DAG level); prefer require.JSONEq when the order is deterministic.
func requireJSONEqualIgnoringArrayOrder(
	t *testing.T,
	expected, actual string,
	msgAndArgs ...any,
) bool {
	t.Helper()

	patch, err := jsondiff.CompareJSON([]byte(expected), []byte(actual), jsondiff.Equivalent())
	require.NoErrorf(t, err, fmt.Sprintf("Error comparing JSON strings: %v", err), msgAndArgs...)
	require.Emptyf(
		t,
		patch,
		fmt.Sprintf("JSON strings are not equal\nExpected: %s\nActual: %s", expected, actual),
		msgAndArgs...)

	return true
}
