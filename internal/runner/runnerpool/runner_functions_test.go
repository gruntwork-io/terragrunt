package runnerpool_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestLogUnitDeployOrder_Flat(t *testing.T) {
	t.Parallel()

	runner := buildTestRunner(t, "/tmp/test", []string{"/tmp/test/vpc", "/tmp/test/app"})
	l := thlogger.CreateLogger()

	err := runner.LogUnitDeployOrder(l, false, false, nil)
	require.NoError(t, err)
}

func TestLogUnitDeployOrder_Destroy(t *testing.T) {
	t.Parallel()

	runner := buildTestRunner(t, "/tmp/test", []string{"/tmp/test/vpc", "/tmp/test/app"})
	l := thlogger.CreateLogger()

	err := runner.LogUnitDeployOrder(l, true, false, nil)
	require.NoError(t, err)
}

func TestJSONUnitDeployOrder(t *testing.T) {
	t.Parallel()

	runner := buildTestRunner(t, "/tmp/test", []string{"/tmp/test/vpc", "/tmp/test/app"})

	result, err := runner.JSONUnitDeployOrder(false, true)
	require.NoError(t, err)
	assert.Contains(t, result, "/tmp/test/vpc")
	assert.Contains(t, result, "/tmp/test/app")
}

func TestJSONUnitDeployOrder_Destroy(t *testing.T) {
	t.Parallel()

	runner := buildTestRunner(t, "/tmp/test", []string{"/tmp/test/vpc", "/tmp/test/app"})

	result, err := runner.JSONUnitDeployOrder(true, false)
	require.NoError(t, err)
	assert.Contains(t, result, "vpc")
	assert.Contains(t, result, "app")
}

func TestListStackDependentUnits_NoDeps(t *testing.T) {
	t.Parallel()

	runner := buildTestRunner(t, "/tmp/test", []string{"/tmp/test/vpc"})

	deps := runner.ListStackDependentUnits()
	assert.Empty(t, deps)
}

func TestListStackDependentUnits_WithDeps(t *testing.T) {
	t.Parallel()

	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})
	app := component.NewUnit("/tmp/test/app").WithConfig(&config.TerragruntConfig{})
	app.AddDependency(vpc)

	runner := buildTestRunnerFromUnits(t, "/tmp/test", component.Components{vpc, app})

	deps := runner.ListStackDependentUnits()
	require.Contains(t, deps, "/tmp/test/vpc")
	assert.Contains(t, deps["/tmp/test/vpc"], "/tmp/test/app")
}

func TestFilterDiscoveredUnits_ExcludesExcluded(t *testing.T) {
	t.Parallel()

	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})
	app := component.NewUnit("/tmp/test/app").WithConfig(&config.TerragruntConfig{})
	app.SetExcluded(true)

	units := []*component.Unit{vpc, app}
	discovered := component.Components{vpc, app}

	filtered := runnerpool.FilterDiscoveredUnits(discovered, units)
	require.Len(t, filtered, 1)
	assert.Equal(t, "/tmp/test/vpc", filtered[0].Path())
}

func TestFilterDiscoveredUnits_AllIncluded(t *testing.T) {
	t.Parallel()

	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})
	app := component.NewUnit("/tmp/test/app").WithConfig(&config.TerragruntConfig{})

	units := []*component.Unit{vpc, app}
	discovered := component.Components{vpc, app}

	filtered := runnerpool.FilterDiscoveredUnits(discovered, units)
	require.Len(t, filtered, 2)
}

func TestFilterDiscoveredUnits_Empty(t *testing.T) {
	t.Parallel()

	filtered := runnerpool.FilterDiscoveredUnits(nil, nil)
	assert.Empty(t, filtered)
}

func TestNewRunnerPoolStack_Empty(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
	require.NoError(t, err)

	l := thlogger.CreateLogger()

	runner, err := runnerpool.NewRunnerPoolStack(
		context.Background(),
		l,
		opts,
		component.Components{},
	)
	require.NoError(t, err)
	require.NotNil(t, runner)

	stack := runner.GetStack()
	assert.Empty(t, stack.Units)
}

func TestLogUnitDeployOrder_DAGExperiment(t *testing.T) {
	t.Parallel()

	runner := buildTestRunner(t, "/tmp/test", []string{"/tmp/test/vpc", "/tmp/test/app"})
	l := thlogger.CreateLogger()

	exps := experiment.NewExperiments()

	err := runner.LogUnitDeployOrder(l, false, false, exps)
	require.NoError(t, err)
}

func TestNewRunnerPoolStack_WithPreventDestroy(t *testing.T) {
	t.Parallel()

	prevent := true
	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{
		PreventDestroy: &prevent,
	})
	app := component.NewUnit("/tmp/test/app").WithConfig(&config.TerragruntConfig{})
	app.AddDependency(vpc)

	opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
	require.NoError(t, err)

	opts.TerraformCommand = "destroy"

	l := thlogger.CreateLogger()

	runner, err := runnerpool.NewRunnerPoolStack(
		context.Background(),
		l,
		opts,
		component.Components{vpc, app},
	)
	require.NoError(t, err)
	require.NotNil(t, runner)

	// vpc should be excluded due to prevent_destroy
	stack := runner.GetStack()

	foundVPC := false

	for _, u := range stack.Units {
		if u.Path() == "/tmp/test/vpc" {
			foundVPC = true

			assert.True(t, u.Excluded(), "vpc should be excluded due to prevent_destroy")
		}
	}

	require.True(t, foundVPC, "expected /tmp/test/vpc unit in stack")
}

func TestNewRunnerPoolStack_FilterAllowDestroy(t *testing.T) {
	t.Parallel()

	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})
	vpc.SetDiscoveryContext(&component.DiscoveryContext{
		Ref:  "abc123",
		Cmd:  "apply",
		Args: []string{"-destroy"},
	})

	opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
	require.NoError(t, err)

	opts.TerraformCommand = "apply"
	opts.FilterAllowDestroy = false

	l := thlogger.CreateLogger()

	runner, err := runnerpool.NewRunnerPoolStack(
		context.Background(),
		l,
		opts,
		component.Components{vpc},
	)
	require.NoError(t, err)

	stack := runner.GetStack()

	foundVPC := false

	for _, u := range stack.Units {
		if u.Path() == "/tmp/test/vpc" {
			foundVPC = true

			assert.True(
				t,
				u.Excluded(),
				"vpc should be excluded: destroy with git ref but no --filter-allow-destroy",
			)
		}
	}

	require.True(t, foundVPC, "expected /tmp/test/vpc unit in stack")
}

func TestFilterDiscoveredUnits_PrunesAndAugments(t *testing.T) {
	t.Parallel()

	const root = "/tmp/test"

	unitPath := func(name string) string { return filepath.Join(root, name) }

	// Resolved units: old is excluded, app was never discovered.
	vpc := component.NewUnit(unitPath("vpc"))
	db := component.NewUnit(unitPath("db"))
	old := component.NewUnit(unitPath("old"))
	app := component.NewUnit(unitPath("app"))

	old.SetExcluded(true)
	db.AddDependency(vpc)
	app.AddDependency(db)
	app.AddDependency(old)
	app.AddDependency(component.NewStack(unitPath("stack")))

	// Discovered components: a stack that must be dropped, an external vpc, and
	// a db whose dependency on the excluded unit must be pruned.
	discVPC := component.NewUnit(unitPath("vpc"))
	discVPC.SetExternal()
	discVPC.SetReading("shared.hcl")

	discDB := component.NewUnit(unitPath("db"))
	discDB.AddDependency(component.NewUnit(unitPath("vpc")))
	discDB.AddDependency(component.NewUnit(unitPath("old")))

	filtered := runnerpool.FilterDiscoveredUnits(
		component.Components{component.NewStack(unitPath("stack")), discVPC, discDB},
		[]*component.Unit{vpc, db, old, app},
	)

	byPath := make(map[string]component.Component, len(filtered))
	for _, c := range filtered {
		byPath[c.Path()] = c
	}

	require.Len(t, byPath, 3)
	require.Contains(t, byPath, unitPath("vpc"))
	require.Contains(t, byPath, unitPath("db"))
	require.Contains(t, byPath, unitPath("app"))

	assert.True(t, byPath[unitPath("vpc")].External())
	assert.Equal(t, []string{"shared.hcl"}, byPath[unitPath("vpc")].Reading())

	assert.Equal(t, []string{unitPath("vpc")}, dependencyPaths(byPath[unitPath("db")]))
	assert.Equal(t, []string{unitPath("db")}, dependencyPaths(byPath[unitPath("app")]))
}

func TestNewRunnerPoolStack_PreventDestroyExcludesDependencies(t *testing.T) {
	t.Parallel()

	prevent := true
	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})
	app := component.NewUnit("/tmp/test/app").WithConfig(&config.TerragruntConfig{
		PreventDestroy: &prevent,
	})
	app.AddDependency(vpc)

	opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
	require.NoError(t, err)

	opts.TerraformCommand = "destroy"

	runner, err := runnerpool.NewRunnerPoolStack(
		context.Background(),
		thlogger.CreateLogger(),
		opts,
		component.Components{vpc, app},
	)
	require.NoError(t, err)

	for _, unit := range runner.GetStack().Units {
		assert.True(t, unit.Excluded(), "%s should be excluded", unit.Path())
	}
}

func TestNewRunnerPoolStack_DestroyWithoutProtectedUnits(t *testing.T) {
	t.Parallel()

	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})

	opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
	require.NoError(t, err)

	opts.TerraformCommand = "destroy"

	runner, err := runnerpool.NewRunnerPoolStack(
		context.Background(),
		thlogger.CreateLogger(),
		opts,
		component.Components{vpc},
	)
	require.NoError(t, err)
	assert.False(t, runner.GetStack().Units[0].Excluded())
}

func TestNewRunnerPoolStack_FilterAllowDestroyKeepsUnit(t *testing.T) {
	t.Parallel()

	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})
	vpc.SetDiscoveryContext(&component.DiscoveryContext{
		Ref:  "abc123",
		Cmd:  "apply",
		Args: []string{"-destroy"},
	})

	opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
	require.NoError(t, err)

	opts.TerraformCommand = "apply"
	opts.FilterAllowDestroy = true

	runner, err := runnerpool.NewRunnerPoolStack(
		context.Background(),
		thlogger.CreateLogger(),
		opts,
		component.Components{vpc},
	)
	require.NoError(t, err)
	assert.False(t, runner.GetStack().Units[0].Excluded())
}

func TestNewRunnerPoolStack_UnitWithoutParsedConfig(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
	require.NoError(t, err)

	runner, err := runnerpool.NewRunnerPoolStack(
		context.Background(),
		thlogger.CreateLogger(),
		opts,
		component.Components{component.NewUnit("/tmp/test/vpc")},
	)
	require.NoError(t, err)
	assert.Len(t, runner.GetStack().Units, 1)
}

func TestListStackDependentUnits_Transitive(t *testing.T) {
	t.Parallel()

	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})
	db := component.NewUnit("/tmp/test/db").WithConfig(&config.TerragruntConfig{})
	app := component.NewUnit("/tmp/test/app").WithConfig(&config.TerragruntConfig{})

	db.AddDependency(vpc)
	app.AddDependency(db)

	runner := buildTestRunnerFromUnits(t, "/tmp/test", component.Components{vpc, db, app})

	deps := runner.ListStackDependentUnits()
	assert.ElementsMatch(t, []string{"/tmp/test/db", "/tmp/test/app"}, deps["/tmp/test/vpc"])
	assert.Equal(t, []string{"/tmp/test/app"}, deps["/tmp/test/db"])
}

// dependencyPaths returns the paths of a component's direct dependencies.
func dependencyPaths(c component.Component) []string {
	paths := make([]string, 0, len(c.Dependencies()))
	for _, dep := range c.Dependencies() {
		paths = append(paths, dep.Path())
	}

	return paths
}

// buildTestRunner creates a Runner with simple unit components for testing.
func buildTestRunner(t *testing.T, workDir string, unitPaths []string) *runnerpool.Runner {
	t.Helper()

	components := make(component.Components, 0, len(unitPaths))
	for _, p := range unitPaths {
		components = append(components, component.NewUnit(p).WithConfig(&config.TerragruntConfig{}))
	}

	return buildTestRunnerFromUnits(t, workDir, components)
}

// buildTestRunnerFromUnits creates a Runner from pre-built unit components.
func buildTestRunnerFromUnits(
	t *testing.T,
	workDir string,
	components component.Components,
) *runnerpool.Runner {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(workDir, "terragrunt.hcl"))
	require.NoError(t, err)

	l := thlogger.CreateLogger()

	runner, err := runnerpool.NewRunnerPoolStack(context.Background(), l, opts, components)
	require.NoError(t, err)

	return runner.(*runnerpool.Runner)
}
