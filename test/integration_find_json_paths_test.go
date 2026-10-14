package test_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFixtureFindJSONPaths = "fixtures/find-json-paths"

// TestFindJSONRelativizesEveryPathField pins every path-bearing field of
// find --json in one run: path, include, dependencies, and reading. The working
// directory is a subdirectory of the fixture, so each field carries an entry
// above it, and --external pulls in a unit that lives outside it. The reading
// entries come from an included root whose relative source resolves against
// the unit, and from a unit whose source is declared in its own config.
func TestFindJSONRelativizesEveryPathField(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindJSONPaths)

	workingDir := filepath.Join(testFixtureFindJSONPaths, "live")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt find --no-color --json --dependencies --include --reading --external --working-dir "+workingDir,
	)
	require.NoError(t, err)
	assert.Empty(t, stderr)

	rel := filepath.FromSlash

	var found find.FoundComponents
	require.NoError(t, json.Unmarshal([]byte(stdout), &found))

	for _, c := range found {
		slices.Sort(c.Dependencies)
		slices.Sort(c.Reading)
	}

	assert.Equal(t, find.FoundComponents{
		{
			Type:    component.UnitKind,
			Path:    rel("../ext/other"),
			Reading: []string{rel("../modules/foo/main.tf")},
		},
		{
			Type:         component.UnitKind,
			Path:         "app",
			Include:      map[string]string{"root": rel("../root.hcl")},
			Dependencies: []string{rel("../ext/other"), "vpc"},
			Reading:      []string{rel("../modules/foo/main.tf"), rel("../root.hcl")},
		},
		{
			Type:    component.UnitKind,
			Path:    "vpc",
			Include: map[string]string{"root": rel("../root.hcl")},
			Reading: []string{rel("../modules/foo/main.tf"), rel("../root.hcl")},
		},
	}, found)
}
