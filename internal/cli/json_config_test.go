package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDirectoryWithBothDefaultConfigs pins that a command that loads a unit from a directory holding both
// terragrunt.hcl and terragrunt.hcl.json reads terragrunt.hcl. Each terragrunt.hcl.json fails to parse.
func TestDirectoryWithBothDefaultConfigs(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")

	tests := []struct {
		files map[string]string
		name  string
		args  []string
	}{
		{
			name: "unit in the working directory",
			files: map[string]string{
				"unit/terragrunt.hcl":      `inputs = { from = "hcl" }`,
				"unit/terragrunt.hcl.json": "{",
			},
			args: []string{"render", "--working-dir", filepath.Join(root, "unit")},
		},
		{
			name: "dependency of the unit in the working directory",
			files: map[string]string{
				"app/terragrunt.hcl":      "dependency \"dep\" {\n  config_path = \"../dep\"\n  skip_outputs = true\n}\n",
				"dep/terragrunt.hcl":      "",
				"dep/terragrunt.hcl.json": "{",
			},
			args: []string{"render", "--working-dir", filepath.Join(root, "app")},
		},
		{
			name: "dependency outside the working directory",
			files: map[string]string{
				"app/terragrunt.hcl":      "dependency \"dep\" {\n  config_path = \"../dep\"\n}\n",
				"dep/terragrunt.hcl":      "",
				"dep/terragrunt.hcl.json": "{",
			},
			args: []string{
				"find", "--dag", "--dependencies", "--external", "--working-dir", filepath.Join(root, "app"),
			},
		},
		{
			name: "dependent above the working directory",
			files: map[string]string{
				".git":                     "gitdir: /elsewhere/.git\n",
				"svc/vpc/terragrunt.hcl":   "",
				"dual/terragrunt.hcl":      "dependency \"vpc\" {\n  config_path = \"../svc/vpc\"\n}\n",
				"dual/terragrunt.hcl.json": "{",
			},
			args: []string{"find", "--filter", "...vpc", "--working-dir", filepath.Join(root, "svc")},
		},
		{
			name: "generated unit of a stack dependency",
			files: map[string]string{
				"app/terragrunt.hcl":                            "dependency \"stack\" {\n  config_path = \"../stack\"\n}\n",
				"stack/terragrunt.stack.hcl":                    "unit \"u\" {\n  source = \"../src\"\n  path = \"u\"\n}\n",
				"stack/.terragrunt-stack/u/terragrunt.hcl":      "",
				"stack/.terragrunt-stack/u/terragrunt.hcl.json": "{",
			},
			args: []string{"render", "--working-dir", filepath.Join(root, "app")},
		},
		{
			name: "dependency outside the hcl validate walk",
			files: map[string]string{
				"live/app/terragrunt.hcl": "dependency \"dep\" {\n  config_path = \"../../dep\"\n}\n",
				"dep/terragrunt.hcl":      "",
				"dep/terragrunt.hcl.json": "{",
			},
			args: []string{"hcl", "validate", "--working-dir", filepath.Join(root, "live")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv().WithFS(venvtest.NewFS(t, root, tt.files))

			_, err := runCLI(t, v, tt.args...)
			require.NoError(t, err)
		})
	}
}

// TestHCLFmtInDirectoryWithBothDefaultConfigs pins that hcl fmt, which loads no unit, runs in a directory
// holding both terragrunt.hcl and terragrunt.hcl.json.
func TestHCLFmtInDirectoryWithBothDefaultConfigs(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")

	v := venv.MemVenv().WithFS(venvtest.NewFS(t, root, map[string]string{
		"unit/terragrunt.hcl":      "inputs = {\n  from = \"hcl\"\n}\n",
		"unit/terragrunt.hcl.json": `{"inputs": {"from": "json"}}`,
	}))

	_, err := runCLI(t, v, "hcl", "fmt", "--check", "--working-dir", filepath.Join(root, "unit"))
	require.NoError(t, err)
}

// TestRenderReadsTerragruntHCLBesideTheJSONConfig pins that render prints terragrunt.hcl from a directory holding
// both terragrunt.hcl and terragrunt.hcl.json.
func TestRenderReadsTerragruntHCLBesideTheJSONConfig(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")

	v := venv.MemVenv().WithFS(venvtest.NewFS(t, root, map[string]string{
		"unit/terragrunt.hcl":      `inputs = { from = "hcl" }`,
		"unit/terragrunt.hcl.json": `{"inputs": {"from": "json"}}`,
	}))

	out, err := runCLI(t, v, "render", "--no-color", "--working-dir", filepath.Join(root, "unit"))
	require.NoError(t, err)

	assert.Equal(t, "inputs = {\n  from = \"hcl\"\n}\n", out)
}

// TestConfigFlagSelectsTheJSONConfigBesideTerragruntHCL pins that --config loads terragrunt.hcl.json from a
// directory that also holds terragrunt.hcl.
func TestConfigFlagSelectsTheJSONConfigBesideTerragruntHCL(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")
	unitDir := filepath.Join(root, "unit")

	v := venv.MemVenv().WithFS(venvtest.NewFS(t, root, map[string]string{
		"unit/terragrunt.hcl":      `inputs = { from = "hcl" }`,
		"unit/terragrunt.hcl.json": `{"inputs": {"from": "json"}}`,
	}))

	out, err := runCLI(
		t, v, "render", "--no-color", "--working-dir", unitDir, "--config", filepath.Join(unitDir, "terragrunt.hcl.json"),
	)
	require.NoError(t, err)

	assert.Equal(t, "inputs = {\n  from = \"json\"\n}\n", out)
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
