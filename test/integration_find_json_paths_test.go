package test_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

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

	expected, err := json.Marshal([]map[string]any{
		{
			"type":    "unit",
			"path":    rel("../ext/other"),
			"reading": []string{rel("../modules/foo/main.tf")},
		},
		{
			"type":         "unit",
			"path":         "app",
			"include":      map[string]string{"root": rel("../root.hcl")},
			"dependencies": []string{"vpc", rel("../ext/other")},
			"reading":      []string{rel("../modules/foo/main.tf"), rel("../root.hcl")},
		},
		{
			"type":    "unit",
			"path":    "vpc",
			"include": map[string]string{"root": rel("../root.hcl")},
			"reading": []string{rel("../modules/foo/main.tf"), rel("../root.hcl")},
		},
	})
	require.NoError(t, err)

	requireJSONEqualIgnoringArrayOrder(t, string(expected), stdout)
}
