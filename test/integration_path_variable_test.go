package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const (
	testFixturePathVariable    = "fixtures/path-variable"
	pathVariableExperimentFlag = " --experiment path-variable"
)

func TestPathVariableInUnitIncludeAndRead(t *testing.T) {
	t.Parallel()

	root := copyPathVariableFixture(t)
	unitDir := filepath.Join(root, "live", "app")

	assert.Equal(t, map[string]any{
		"unit":         unitDir,
		"include":      "not-included",
		"read":         "not-read",
		"stack":        "no-stack",
		"env_read":     filepath.Join(root, "env.hcl"),
		"env_unit":     unitDir,
		"env_include":  "not-included",
		"shared_read":  filepath.Join(root, "shared.hcl"),
		"root_unit":    unitDir,
		"root_include": filepath.Join(root, "root.hcl"),
		"root_local":   filepath.Join(root, "root.hcl"),
		"root_read":    "not-read",
	}, renderPathVariableInputs(t, unitDir))
}

func TestPathVariableModuleMatchesGetWorkingDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		unit string
	}{
		{name: "source in terraform block", unit: "remote"},
		{name: "no source", unit: "local"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := copyPathVariableFixture(t)
			inputs := renderPathVariableInputs(t, filepath.Join(root, "module", tc.unit))

			require.NotEmpty(t, inputs["working_dir"])
			assert.Equal(t, inputs["working_dir"], inputs["module"])
		})
	}
}

func TestPathVariableModuleRejectsDependencyReference(t *testing.T) {
	t.Parallel()

	root := copyPathVariableFixture(t)
	unitDir := filepath.Join(root, "module", "dependency", "download-dir")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt render --format json --non-interactive --working-dir "+unitDir+pathVariableExperimentFlag,
	)

	var got config.ModuleDirReferencesDependencyError

	require.ErrorAs(t, err, &got)
	assert.Equal(t, "download_dir", got.Attribute)
}

func TestPathVariableInRenderAll(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		dir   string
		units []string
	}{
		{name: "unit, include and read", dir: "live", units: []string{"absolute", "app", "db", "json"}},
		{
			name:  "module",
			dir:   "module",
			units: []string{"download-dir", "download-dir-include", "in-place", "local", "remote"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := copyPathVariableFixture(t)
			workingDir := filepath.Join(root, tc.dir)
			want := map[string]map[string]any{}

			for _, unit := range tc.units {
				want[unit] = renderPathVariableInputs(t, filepath.Join(workingDir, unit))
			}

			helpers.RunTerragrunt(
				t,
				"terragrunt render --all --format json --write --non-interactive --working-dir "+workingDir+
					" --filter '!./dependency/**'"+pathVariableExperimentFlag,
			)

			got := map[string]map[string]any{}

			for _, unit := range tc.units {
				got[unit] = readRenderedPathVariableInputs(t, filepath.Join(workingDir, unit))
			}

			assert.Equal(t, want, got)
		})
	}
}

func TestPathVariableUnitInBackendDeleteAll(t *testing.T) {
	t.Parallel()

	root := copyPathVariableFixture(t)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt backend delete --all --force --non-interactive --working-dir "+filepath.Join(root, "all")+
			pathVariableExperimentFlag,
	)
	require.NoError(t, err)

	assert.Equal(t, 2, strings.Count(stderr, "Delete for local backend not implemented"))
}

func TestPathVariableInGeneratedStack(t *testing.T) {
	t.Parallel()

	root := copyPathVariableFixture(t)
	stackDir := filepath.Join(root, "stack", "live")
	unitDir := filepath.Join(stackDir, ".terragrunt-stack", "app")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+stackDir+pathVariableExperimentFlag)

	assert.Equal(t, map[string]any{
		"unit":            unitDir,
		"stack":           "no-stack",
		"stack_dir":       stackDir,
		"unit_in_stack":   "no-unit",
		"module_in_stack": "no-module",
	}, renderPathVariableInputs(t, unitDir))
}

func TestPathVariableInGeneratedAutoInclude(t *testing.T) {
	t.Parallel()

	root := copyPathVariableFixture(t)
	stackDir := filepath.Join(root, "autoinclude", "live")
	unitDir := filepath.Join(stackDir, ".terragrunt-stack", "app")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+stackDir+pathVariableExperimentFlag)

	inputs := renderPathVariableInputs(t, unitDir)

	require.NotEmpty(t, inputs["working_dir"])
	assert.Equal(t, map[string]any{
		"unit":        unitDir,
		"working_dir": inputs["working_dir"],
		"auto_stack":  stackDir,
		"auto_unit":   "no-unit",
		"auto_label":  "live:" + stackDir,
		"db_consumer": stackDir,
	}, inputs)
}

func TestPathVariableInGeneratedStackInclude(t *testing.T) {
	t.Parallel()

	root := copyPathVariableFixture(t)
	stackDir := filepath.Join(root, "stack-include", "live")
	includePath := filepath.Join(root, "stack-include", "catalog", "components.hcl")
	rootDir := filepath.Join(stackDir, ".terragrunt-stack", "root")
	appDir := filepath.Join(stackDir, ".terragrunt-stack", "catalog-app")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+stackDir+pathVariableExperimentFlag)

	assert.Equal(t, map[string]any{
		"unit":      rootDir,
		"include":   "no-include",
		"stack_dir": stackDir,
	}, renderPathVariableInputs(t, rootDir))
	assert.Equal(t, map[string]any{
		"unit":         appDir,
		"include":      includePath,
		"stack_dir":    stackDir,
		"auto_include": includePath,
		"auto_index":   includePath,
		"auto_guard":   "components.hcl",
		"auto_label":   "components.hcl:live",
	}, renderPathVariableInputs(t, appDir))
}

func TestPathVariableInStackIncludeDiscovery(t *testing.T) {
	t.Parallel()

	root := copyPathVariableFixture(t)
	stackRoot := filepath.Join(root, "stack-include")

	assert.Equal(t, map[string][]string{
		"consumer": {"live/.terragrunt-stack/root", "live/.terragrunt-stack/catalog-app"},
	}, findPathVariableDependencies(t, stackRoot))
}

func TestPathVariableInStackDependencyDiscovery(t *testing.T) {
	t.Parallel()

	root := copyPathVariableFixture(t)
	stackRoot := filepath.Join(root, "stack")

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --working-dir "+filepath.Join(stackRoot, "live")+pathVariableExperimentFlag,
	)

	assert.Equal(
		t,
		map[string][]string{"consumer": {"live/.terragrunt-stack/app"}},
		findPathVariableDependencies(t, stackRoot),
	)
}

func TestPathVariableRequiresExperiment(t *testing.T) {
	t.Parallel()

	helpers.SkipInExperimentMode(t, experiment.PathVariable)

	root := copyPathVariableFixture(t)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt render --format json --non-interactive --working-dir "+filepath.Join(root, "live", "json"),
	)
	require.ErrorContains(t, err, "--experiment "+experiment.PathVariable)
}

// findPathVariableDependencies returns the dependencies of each component that discovery finds in workingDir.
func findPathVariableDependencies(t *testing.T, workingDir string) map[string][]string {
	t.Helper()

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt find --dag --dependencies --json --non-interactive --working-dir "+workingDir+
			pathVariableExperimentFlag,
	)
	require.NoError(t, err)

	var components []struct {
		Path         string   `json:"path"`
		Dependencies []string `json:"dependencies"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &components))

	dependencies := map[string][]string{}

	for _, component := range components {
		for _, dependency := range component.Dependencies {
			path := filepath.ToSlash(component.Path)
			dependencies[path] = append(dependencies[path], filepath.ToSlash(dependency))
		}
	}

	return dependencies
}

// copyPathVariableFixture copies the path-variable fixture to a temporary directory and returns its resolved path.
func copyPathVariableFixture(t *testing.T) string {
	t.Helper()

	tmpEnvPath, err := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixturePathVariable))
	require.NoError(t, err)

	return filepath.Join(tmpEnvPath, testFixturePathVariable)
}

// renderPathVariableInputs renders the unit in unitDir with the path-variable experiment and returns its inputs.
func renderPathVariableInputs(t *testing.T, unitDir string) map[string]any {
	t.Helper()

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt render --format json --non-interactive --working-dir "+unitDir+pathVariableExperimentFlag,
	)
	require.NoError(t, err)

	return pathVariableInputs(t, []byte(stdout))
}

// readRenderedPathVariableInputs returns the inputs of the rendered JSON file that `render --write` left in unitDir.
func readRenderedPathVariableInputs(t *testing.T, unitDir string) map[string]any {
	t.Helper()

	rendered, err := os.ReadFile(filepath.Join(unitDir, "terragrunt.rendered.json"))
	require.NoError(t, err)

	return pathVariableInputs(t, rendered)
}

// pathVariableInputs returns the inputs of a rendered JSON config.
func pathVariableInputs(t *testing.T, renderedJSON []byte) map[string]any {
	t.Helper()

	var rendered struct {
		Inputs map[string]any `json:"inputs"`
	}

	require.NoError(t, json.Unmarshal(renderedJSON, &rendered))

	return rendered.Inputs
}
