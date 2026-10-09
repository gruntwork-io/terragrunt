package test_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/internal/component"
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
		"terragrunt find --no-color --dependencies --json --working-dir "+
			testFixtureDependencyExpansionKeyed,
	)
	require.NoError(t, err)

	assert.Empty(t, stderr)

	var found find.FoundComponents
	require.NoError(t, json.Unmarshal([]byte(stdout), &found))

	// find sorts units by path but lists each unit's dependencies in discovery order.
	for _, c := range found {
		slices.Sort(c.Dependencies)
	}

	assert.Equal(t, find.FoundComponents{
		{
			Type:         component.UnitKind,
			Path:         "app",
			Dependencies: []string{"aurora-api", "aurora-web", "shard-0", "shard-1", "vpc"},
		},
		{Type: component.UnitKind, Path: "aurora-api"},
		{Type: component.UnitKind, Path: "aurora-web"},
		{Type: component.UnitKind, Path: "shard-0"},
		{Type: component.UnitKind, Path: "shard-1"},
		{Type: component.UnitKind, Path: "vpc"},
	}, found)
}
