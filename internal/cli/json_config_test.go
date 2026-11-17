package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAmbiguousDefaultConfig pins that a command run in a directory holding both
// terragrunt.hcl and terragrunt.hcl.json is rejected instead of picking one.
func TestAmbiguousDefaultConfig(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")
	unitDir := filepath.Join(root, "unit")

	v := venv.MemVenv().WithFS(venvtest.NewFS(t, root, map[string]string{
		"unit/terragrunt.hcl":      `inputs = { from = "hcl" }`,
		"unit/terragrunt.hcl.json": `{"inputs": {"from": "json"}}`,
	}))

	_, err := runCLI(t, v, "render", "--working-dir", unitDir)

	var ambiguousErr discovery.AmbiguousConfigError
	require.ErrorAs(t, err, &ambiguousErr)
	assert.Equal(t, unitDir, ambiguousErr.ComponentPath)
}

// TestRenderJSONOnlyUnit pins that a directory holding only terragrunt.hcl.json still renders.
func TestRenderJSONOnlyUnit(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")

	v := venv.MemVenv().WithFS(venvtest.NewFS(t, root, map[string]string{
		"unit/terragrunt.hcl.json": `{"inputs": {"from": "json"}}`,
	}))

	out, err := runCLI(t, v, "render", "--no-color", "--working-dir", filepath.Join(root, "unit"))
	require.NoError(t, err)

	assert.Equal(t, "inputs = {\n  from = \"json\"\n}\n", out)
}

// TestHCLValidateJSONOnlyUnit pins that hcl validate reads a unit from terragrunt.hcl.json when
// other units in the tree use terragrunt.hcl.
func TestHCLValidateJSONOnlyUnit(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")

	v := venv.MemVenv().WithFS(venvtest.NewFS(t, root, map[string]string{
		"json-unit/terragrunt.hcl.json": `{"inputs": {"from": "json"}}`,
		"hcl-unit/terragrunt.hcl":       `inputs = { from = "hcl" }`,
	}))

	_, err := runCLI(t, v, "hcl", "validate", "--working-dir", root)
	require.NoError(t, err)
}
