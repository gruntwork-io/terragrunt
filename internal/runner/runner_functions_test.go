package runner_test

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestLogUnitDeployOrder_Flat(t *testing.T) {
	t.Parallel()

	rnr := buildTestRunner(t, "/tmp/test", []string{"/tmp/test/vpc", "/tmp/test/app"})
	l := thlogger.CreateLogger()

	err := rnr.LogUnitDeployOrder(l, venvtest.New(), false, false, nil)
	require.NoError(t, err)
}

func TestLogUnitDeployOrder_Destroy(t *testing.T) {
	t.Parallel()

	rnr := buildTestRunner(t, "/tmp/test", []string{"/tmp/test/vpc", "/tmp/test/app"})
	l := thlogger.CreateLogger()

	err := rnr.LogUnitDeployOrder(l, venvtest.New(), true, false, nil)
	require.NoError(t, err)
}

func TestJSONUnitDeployOrder(t *testing.T) {
	t.Parallel()

	rnr := buildTestRunner(t, "/tmp/test", []string{"/tmp/test/vpc", "/tmp/test/app"})

	result, err := rnr.JSONUnitDeployOrder(false, true)
	require.NoError(t, err)
	assert.Contains(t, result, "/tmp/test/vpc")
	assert.Contains(t, result, "/tmp/test/app")
}

func TestJSONUnitDeployOrder_Destroy(t *testing.T) {
	t.Parallel()

	rnr := buildTestRunner(t, "/tmp/test", []string{"/tmp/test/vpc", "/tmp/test/app"})

	result, err := rnr.JSONUnitDeployOrder(true, false)
	require.NoError(t, err)
	assert.Contains(t, result, "vpc")
	assert.Contains(t, result, "app")
}

func TestListStackDependentUnits_NoDeps(t *testing.T) {
	t.Parallel()

	rnr := buildTestRunner(t, "/tmp/test", []string{"/tmp/test/vpc"})

	deps := rnr.ListStackDependentUnits()
	assert.Empty(t, deps)
}

func TestListStackDependentUnits_WithDeps(t *testing.T) {
	t.Parallel()

	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})
	app := component.NewUnit("/tmp/test/app").WithConfig(&config.TerragruntConfig{})
	app.AddDependency(vpc)

	rnr := buildTestRunnerFromUnits(t, "/tmp/test", component.Components{vpc, app})

	deps := rnr.ListStackDependentUnits()
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

	filtered := runner.FilterDiscoveredUnits(discovered, units)
	require.Len(t, filtered, 1)
	assert.Equal(t, "/tmp/test/vpc", filtered[0].Path())
}

func TestFilterDiscoveredUnits_AllIncluded(t *testing.T) {
	t.Parallel()

	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})
	app := component.NewUnit("/tmp/test/app").WithConfig(&config.TerragruntConfig{})

	units := []*component.Unit{vpc, app}
	discovered := component.Components{vpc, app}

	filtered := runner.FilterDiscoveredUnits(discovered, units)
	require.Len(t, filtered, 2)
}

func TestFilterDiscoveredUnits_Empty(t *testing.T) {
	t.Parallel()

	filtered := runner.FilterDiscoveredUnits(nil, nil)
	assert.Empty(t, filtered)
}

func TestNewFromComponents_Empty(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
	require.NoError(t, err)

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(
		t.Context(),
		l,
		opts,
		component.Components{},
	)
	require.NoError(t, err)
	require.NotNil(t, rnr)

	stack := rnr.GetStack()
	assert.Empty(t, stack.Units)
}

func TestLogUnitDeployOrder_DAGExperiment(t *testing.T) {
	t.Parallel()

	rnr := buildTestRunner(t, "/tmp/test", []string{"/tmp/test/vpc", "/tmp/test/app"})
	l := thlogger.CreateLogger()

	exps := experiment.NewExperiments()

	err := rnr.LogUnitDeployOrder(l, venvtest.New(), false, false, exps)
	require.NoError(t, err)
}

func TestNewFromComponents_WithPreventDestroy(t *testing.T) {
	t.Parallel()

	prevent := true
	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{
		PreventDestroy: &prevent,
	})
	app := component.NewUnit("/tmp/test/app").WithConfig(&config.TerragruntConfig{})
	app.AddDependency(vpc)

	opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
	require.NoError(t, err)

	opts.TerraformCommand = tf.CommandNameDestroy

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(
		t.Context(),
		l,
		opts,
		component.Components{vpc, app},
	)
	require.NoError(t, err)
	require.NotNil(t, rnr)

	// vpc should be excluded due to prevent_destroy
	stack := rnr.GetStack()

	foundVPC := false

	for _, u := range stack.Units {
		if u.Path() == "/tmp/test/vpc" {
			foundVPC = true

			assert.True(t, u.Excluded(), "vpc should be excluded due to prevent_destroy")
		}
	}

	require.True(t, foundVPC, "expected /tmp/test/vpc unit in stack")
}

func TestNewFromComponents_FilterAllowDestroy(t *testing.T) {
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

	rnr, err := runner.NewFromComponents(
		t.Context(),
		l,
		opts,
		component.Components{vpc},
	)
	require.NoError(t, err)

	stack := rnr.GetStack()

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

	filtered := runner.FilterDiscoveredUnits(
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

func TestNewFromComponents_PreventDestroyExcludesDependencies(t *testing.T) {
	t.Parallel()

	prevent := true
	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})
	app := component.NewUnit("/tmp/test/app").WithConfig(&config.TerragruntConfig{
		PreventDestroy: &prevent,
	})
	app.AddDependency(vpc)

	opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
	require.NoError(t, err)

	opts.TerraformCommand = tf.CommandNameDestroy

	rnr, err := runner.NewFromComponents(
		t.Context(),
		thlogger.CreateLogger(),
		opts,
		component.Components{vpc, app},
	)
	require.NoError(t, err)

	for _, unit := range rnr.GetStack().Units {
		assert.True(t, unit.Excluded(), "%s should be excluded", unit.Path())
	}
}

func TestNewFromComponents_DestroyWithoutProtectedUnits(t *testing.T) {
	t.Parallel()

	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})

	opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
	require.NoError(t, err)

	opts.TerraformCommand = tf.CommandNameDestroy

	rnr, err := runner.NewFromComponents(
		t.Context(),
		thlogger.CreateLogger(),
		opts,
		component.Components{vpc},
	)
	require.NoError(t, err)
	assert.False(t, rnr.GetStack().Units[0].Excluded())
}

func TestNewFromComponents_FilterAllowDestroyKeepsUnit(t *testing.T) {
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

	rnr, err := runner.NewFromComponents(
		t.Context(),
		thlogger.CreateLogger(),
		opts,
		component.Components{vpc},
	)
	require.NoError(t, err)
	assert.False(t, rnr.GetStack().Units[0].Excluded())
}

func TestNewFromComponents_UnitWithoutParsedConfig(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
	require.NoError(t, err)

	rnr, err := runner.NewFromComponents(
		t.Context(),
		thlogger.CreateLogger(),
		opts,
		component.Components{component.NewUnit("/tmp/test/vpc")},
	)
	require.NoError(t, err)
	assert.Len(t, rnr.GetStack().Units, 1)
}

func TestListStackDependentUnits_Transitive(t *testing.T) {
	t.Parallel()

	vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})
	db := component.NewUnit("/tmp/test/db").WithConfig(&config.TerragruntConfig{})
	app := component.NewUnit("/tmp/test/app").WithConfig(&config.TerragruntConfig{})

	db.AddDependency(vpc)
	app.AddDependency(db)

	rnr := buildTestRunnerFromUnits(t, "/tmp/test", component.Components{vpc, db, app})

	deps := rnr.ListStackDependentUnits()
	assert.ElementsMatch(t, []string{"/tmp/test/db", "/tmp/test/app"}, deps["/tmp/test/vpc"])
	assert.Equal(t, []string{"/tmp/test/app"}, deps["/tmp/test/db"])
}

func TestNewFromComponents_PreventDestroyWalksTheDependencyGraph(t *testing.T) {
	t.Parallel()

	prevent := true

	testCases := []struct {
		components  func() component.Components
		name        string
		excluded    []string
		notExcluded []string
	}{
		{
			name: "transitive dependencies are excluded",
			components: func() component.Components {
				vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})
				db := component.NewUnit("/tmp/test/db").WithConfig(&config.TerragruntConfig{})
				app := component.NewUnit("/tmp/test/app").WithConfig(&config.TerragruntConfig{
					PreventDestroy: &prevent,
				})

				db.AddDependency(vpc)
				app.AddDependency(db)

				return component.Components{vpc, db, app}
			},
			excluded: []string{"/tmp/test/vpc", "/tmp/test/db", "/tmp/test/app"},
		},
		{
			name: "dependency cycles terminate",
			components: func() component.Components {
				a := component.NewUnit("/tmp/test/a").WithConfig(&config.TerragruntConfig{
					PreventDestroy: &prevent,
				})
				b := component.NewUnit("/tmp/test/b").WithConfig(&config.TerragruntConfig{})

				a.AddDependency(b)
				b.AddDependency(a)

				return component.Components{a, b}
			},
			excluded: []string{"/tmp/test/a", "/tmp/test/b"},
		},
		{
			name: "dependencies that are not units are skipped",
			components: func() component.Components {
				app := component.NewUnit("/tmp/test/app").WithConfig(&config.TerragruntConfig{
					PreventDestroy: &prevent,
				})
				vpc := component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{})

				app.AddDependency(component.NewStack("/tmp/test/stack"))

				return component.Components{app, vpc}
			},
			excluded:    []string{"/tmp/test/app"},
			notExcluded: []string{"/tmp/test/vpc"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
			require.NoError(t, err)

			opts.TerraformCommand = tf.CommandNameDestroy

			rnr, err := runner.NewFromComponents(
				t.Context(),
				thlogger.CreateLogger(),
				opts,
				tc.components(),
			)
			require.NoError(t, err)

			byPath := unitsByPath(rnr)

			for _, path := range tc.excluded {
				require.Contains(t, byPath, path)
				assert.True(t, byPath[path].Excluded(), "%s should be excluded", path)
			}

			for _, path := range tc.notExcluded {
				require.Contains(t, byPath, path)
				assert.False(t, byPath[path].Excluded(), "%s should not be excluded", path)
			}
		})
	}
}

// TestNewFromComponents_PreventDestroyStopsAtTraversalDepth pins that a protected unit only protects dependencies within the traversal bound.
func TestNewFromComponents_PreventDestroyStopsAtTraversalDepth(t *testing.T) {
	t.Parallel()

	const (
		maxDepth = runner.MaxDependencyTraversalDepth
		chainLen = maxDepth + 10
	)

	prevent := true
	components := make(component.Components, chainLen)

	for i := range chainLen {
		cfg := &config.TerragruntConfig{}
		if i == 0 {
			cfg.PreventDestroy = &prevent
		}

		components[i] = component.NewUnit(unitPathAt(i)).WithConfig(cfg)

		if i > 0 {
			components[i-1].AddDependency(components[i])
		}
	}

	opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
	require.NoError(t, err)

	opts.TerraformCommand = tf.CommandNameDestroy

	rnr, err := runner.NewFromComponents(
		t.Context(),
		thlogger.CreateLogger(),
		opts,
		components,
	)
	require.NoError(t, err)

	byPath := unitsByPath(rnr)

	require.Contains(t, byPath, unitPathAt(maxDepth))
	require.Contains(t, byPath, unitPathAt(maxDepth+1))

	assert.True(t, byPath[unitPathAt(maxDepth)].Excluded(), "last unit within the depth bound")
	assert.False(t, byPath[unitPathAt(maxDepth+1)].Excluded(), "first unit beyond the depth bound")
}

// TestNewFromComponents_WithGitRefsAndStateBackends pins stack construction across Git ref and state backend combinations.
func TestNewFromComponents_WithGitRefsAndStateBackends(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		unitConfig       *config.TerragruntConfig
		discoveryContext *component.DiscoveryContext
		name             string
	}{
		{
			name:       "no discovery context",
			unitConfig: &config.TerragruntConfig{},
		},
		{
			name:             "no git ref",
			unitConfig:       &config.TerragruntConfig{},
			discoveryContext: &component.DiscoveryContext{},
		},
		{
			name:             "no parsed config",
			discoveryContext: &component.DiscoveryContext{Ref: "HEAD~1"},
		},
		{
			name: "remote state is configured",
			unitConfig: &config.TerragruntConfig{
				RemoteState: remotestate.New(&remotestate.Config{BackendName: "s3"}),
			},
			discoveryContext: &component.DiscoveryContext{Ref: "HEAD~1"},
		},
		{
			name: "local state is configured",
			unitConfig: &config.TerragruntConfig{
				RemoteState: remotestate.New(&remotestate.Config{BackendName: "local"}),
			},
			discoveryContext: &component.DiscoveryContext{Ref: "HEAD~1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			unit := component.NewUnit("/tmp/test/vpc")
			if tc.unitConfig != nil {
				unit = unit.WithConfig(tc.unitConfig)
			}

			if tc.discoveryContext != nil {
				unit.SetDiscoveryContext(tc.discoveryContext)
			}

			opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
			require.NoError(t, err)

			rnr, err := runner.NewFromComponents(
				t.Context(),
				thlogger.CreateLogger(),
				opts,
				component.Components{unit},
			)
			require.NoError(t, err)
			assert.Len(t, rnr.GetStack().Units, 1, "the unit is kept whatever its state backend")
		})
	}
}

func TestNewFromComponents_IgnoresStackComponents(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("/tmp/test/terragrunt.hcl")
	require.NoError(t, err)

	discovered := component.Components{
		component.NewStack("/tmp/test/stack"),
		component.NewUnit("/tmp/test/vpc").WithConfig(&config.TerragruntConfig{}),
	}

	rnr, err := runner.NewFromComponents(
		t.Context(),
		thlogger.CreateLogger(),
		opts,
		discovered,
	)
	require.NoError(t, err)

	units := rnr.GetStack().Units
	require.Len(t, units, 1)
	assert.Equal(t, "/tmp/test/vpc", units[0].Path())
}

// unitPathAt returns the path of the i-th unit in a dependency chain.
func unitPathAt(i int) string {
	return "/tmp/test/unit" + strconv.Itoa(i)
}

// unitsByPath indexes the units of a runner's stack by path.
func unitsByPath(rnr *runner.Runner) map[string]*component.Unit {
	units := rnr.GetStack().Units
	byPath := make(map[string]*component.Unit, len(units))

	for _, unit := range units {
		byPath[unit.Path()] = unit
	}

	return byPath
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
func buildTestRunner(t *testing.T, workDir string, unitPaths []string) *runner.Runner {
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
) *runner.Runner {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(workDir, "terragrunt.hcl"))
	require.NoError(t, err)

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, components)
	require.NoError(t, err)

	return rnr
}
