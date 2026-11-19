package config_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestPathVariableInUnitIncludeAndRead(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	unitDir := filepath.Join(root, "live", "app")

	inputs := parsePathUnit(t, v, unitDir)

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
	}, inputs)
}

func TestPathVariableReadIsScopedToTheReadingUnit(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	written := time.Now().Add(-time.Hour)

	for _, name := range []string{"shared.hcl", "shared.txt", "env.hcl", "env.txt"} {
		require.NoError(t, v.FS.Chtimes(filepath.Join(root, name), written, written))
	}

	counting := &countingFS{FS: v.FS, opens: map[string]int{}}
	v = v.WithFS(counting)
	ctx := config.WithCaches(t.Context())
	sharedData := filepath.Join(root, "shared.txt")
	envData := filepath.Join(root, "env.txt")

	firstDir := filepath.Join(root, "cache", "one")
	firstPath := filepath.Join(firstDir, config.DefaultTerragruntConfigPath)
	firstInputs := parsePathInputs(ctx, t, v, firstPath)
	sharedOpens, envOpens := counting.opensOf(sharedData), counting.opensOf(envData)

	secondDir := filepath.Join(root, "cache", "two")
	secondPath := filepath.Join(secondDir, config.DefaultTerragruntConfigPath)
	secondInputs := parsePathInputs(ctx, t, v, secondPath)

	assert.Equal(t, firstDir, firstInputs["env_unit"])
	assert.Equal(t, secondDir, secondInputs["env_unit"])
	assert.Equal(t, firstInputs["shared_read"], secondInputs["shared_read"])
	assert.Positive(t, sharedOpens)
	assert.Equal(t, sharedOpens, counting.opensOf(sharedData))
	assert.Greater(t, counting.opensOf(envData), envOpens)
}

func TestPathVariableInNestedRead(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	unitDir := filepath.Join(root, "nested-read", "app")

	inputs := parsePathUnit(t, v, unitDir)

	assert.Equal(t, map[string]any{
		"outer_read": filepath.Join(root, "nested-read", "outer.hcl"),
		"inner_read": filepath.Join(root, "nested-read", "inner.hcl"),
		"inner_unit": unitDir,
		"read":       "not-read",
	}, inputs)
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

			v, root := pathFixture(t)
			inputs := parsePathUnit(t, v, filepath.Join(root, "module", tc.unit))

			require.NotEmpty(t, inputs["working_dir"])
			assert.Equal(t, inputs["working_dir"], inputs["module"])
		})
	}
}

func TestPathVariableAsWholeObject(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	unitDir := filepath.Join(root, "module", "local")

	inputs := parsePathUnit(t, v, unitDir)

	assert.Equal(t, map[string]any{"unit": unitDir, "module": inputs["working_dir"]}, inputs["whole"])
}

func TestPathVariableInValuesFile(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	unitDir := filepath.Join(root, "values", "app")

	inputs := parsePathUnit(t, v, unitDir)

	assert.Equal(t, map[string]any{"from_values": unitDir}, inputs)
}

func TestPathVariableInJSONConfig(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	unitDir := filepath.Join(root, "live", "json")

	cfgPath := filepath.Join(unitDir, config.DefaultTerragruntJSONConfigPath)

	inputs := parsePathInputs(config.WithCaches(t.Context()), t, v, cfgPath)

	assert.Equal(t, map[string]any{"unit": unitDir, "indexed": unitDir}, inputs)
}

func TestPathVariableStackOfUnitBelowStackFile(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "stack", "live")
	unitDir := filepath.Join(stackDir, "manual")

	inputs := parsePathUnit(t, v, unitDir)

	assert.Equal(t, map[string]any{"unit": unitDir, "stack": stackDir}, inputs)
}

func TestPathVariableInStackFile(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "stack", "live")
	stackPath := filepath.Join(stackDir, config.DefaultStackFile)

	ctx, pctx := newPathParsingContext(t, stackPath)

	stack, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), v, pctx, stackPath, nil)
	require.NoError(t, err)
	require.Len(t, stack.Units, 1)
	require.NotNil(t, stack.Units[0].Values)

	values, err := ctyhelper.ParseCtyValueToMap(*stack.Units[0].Values)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"stack_dir":       stackDir,
		"unit_in_stack":   "no-unit",
		"module_in_stack": "no-module",
	}, values)
}

func TestPathVariableInStackFileReadFromUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		stackDir string
	}{
		{name: "stack locals", stackDir: filepath.Join("stack", "live")},
		{name: "stack values file", stackDir: "stack-values"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, root := pathFixture(t)
			stackDir := filepath.Join(root, tc.stackDir)
			stackPath := filepath.Join(stackDir, config.DefaultStackFile)

			ctx, pctx := unitPathParsingContext(t, v, filepath.Join(root, "stack", "consumer"))
			l := logger.CreateLogger()

			values, err := config.ReadValues(ctx, l, v, pctx, stackDir)
			require.NoError(t, err)

			stack, err := config.ReadStackConfigFile(ctx, l, v, pctx, stackPath, values)
			require.NoError(t, err)
			require.Len(t, stack.Units, 1)
			require.NotNil(t, stack.Units[0].Values)

			unitValues, err := ctyhelper.ParseCtyValueToMap(*stack.Units[0].Values)
			require.NoError(t, err)

			assert.Equal(t, map[string]any{
				"stack_dir":       stackDir,
				"unit_in_stack":   "no-unit",
				"module_in_stack": "no-module",
			}, unitValues)
		})
	}
}

func TestPathVariableInStackValuesFileReadByUnit(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "stack-values")

	inputs := parsePathUnit(t, v, filepath.Join(stackDir, "reader"))

	assert.Equal(t, map[string]any{
		"stack_dir":     stackDir,
		"unit_in_stack": "no-unit",
		"read_file":     filepath.Join(stackDir, "terragrunt.values.hcl"),
	}, inputs)
}

func TestPathVariableInAutoIncludeFile(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	unitDir := filepath.Join(root, "autoinclude", "unit")

	inputs := parsePathUnit(t, v, unitDir)

	require.NotEmpty(t, inputs["working_dir"])
	assert.Equal(t, map[string]any{
		"unit":        unitDir,
		"working_dir": inputs["working_dir"],
		"auto_unit":   unitDir,
		"auto_module": inputs["working_dir"],
		"auto_stack":  "no-stack",
	}, inputs)
}

func TestPathVariableInGeneratedAutoInclude(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "autoinclude", "live")
	unitDir := filepath.Join(stackDir, config.StackDir, "app")

	require.NoError(t, generatePathStack(t, v, stackDir, experiment.PathVariable))

	inputs := parsePathUnit(t, v, unitDir)

	require.NotEmpty(t, inputs["working_dir"])
	assert.Equal(t, map[string]any{
		"unit":        unitDir,
		"working_dir": inputs["working_dir"],
		"auto_unit":   unitDir,
		"auto_module": inputs["working_dir"],
		"auto_stack":  stackDir,
		"auto_label":  "live:" + unitDir,
		"db_consumer": unitDir,
	}, inputs)
}

func TestPathVariableInAutoIncludeBlockRequiresExperiment(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)

	err := generatePathStack(t, v, filepath.Join(root, "autoinclude", "live"))

	var unresolved inthclparse.PartialEvalUnresolvedError

	require.ErrorAs(t, err, &unresolved)
}

func TestPathVariableInStackInclude(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "stack-include", "live")
	catalogDir := filepath.Join(root, "stack-include", "catalog")
	stackPath := filepath.Join(stackDir, config.DefaultStackFile)

	ctx, pctx := newPathParsingContext(t, stackPath)

	stack, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), v, pctx, stackPath, nil)
	require.NoError(t, err)
	require.Len(t, stack.Units, 2)

	units := map[string]*config.Unit{}
	values := map[string]map[string]any{}

	for _, unit := range stack.Units {
		require.NotNil(t, unit.Values)

		units[unit.Name] = unit
		values[unit.Name], err = ctyhelper.ParseCtyValueToMap(*unit.Values)
		require.NoError(t, err)
	}

	require.Contains(t, units, "app")
	assert.Equal(t, filepath.ToSlash(filepath.Join(catalogDir, "units", "app")), filepath.ToSlash(units["app"].Source))
	assert.Equal(t, "catalog-app", units["app"].Path)
	assert.Equal(t, map[string]map[string]any{
		"root": {"include": "no-include", "stack_dir": stackDir},
		"app":  {"include": filepath.Join(catalogDir, "components.hcl"), "stack_dir": stackDir},
	}, values)
}

func TestPathVariableInGeneratedStackInclude(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "stack-include", "live")
	includePath := filepath.Join(root, "stack-include", "catalog", "components.hcl")
	rootDir := filepath.Join(stackDir, config.StackDir, "root")
	appDir := filepath.Join(stackDir, config.StackDir, "catalog-app")

	require.NoError(t, generatePathStack(t, v, stackDir, experiment.PathVariable))

	assert.Equal(t, map[string]any{
		"unit":      rootDir,
		"include":   "no-include",
		"stack_dir": stackDir,
	}, parsePathUnit(t, v, rootDir))
	assert.Equal(t, map[string]any{
		"unit":         appDir,
		"include":      includePath,
		"stack_dir":    stackDir,
		"auto_include": includePath,
		"auto_label":   "components.hcl:" + appDir,
	}, parsePathUnit(t, v, appDir))
}

func TestPathVariableInStackFileEvalVariables(t *testing.T) {
	t.Parallel()

	stackDir := venvtest.Root("/repo/stack")

	testCases := []struct {
		want        map[string]cty.Value
		name        string
		experiments []string
	}{
		{name: "experiment disabled", want: map[string]cty.Value{}},
		{
			name:        "experiment enabled",
			experiments: []string{experiment.PathVariable},
			want: map[string]cty.Value{
				config.MetadataPath: cty.ObjectVal(map[string]cty.Value{"stack": cty.StringVal(stackDir)}),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stackPath := filepath.Join(stackDir, config.DefaultStackFile)
			ctx, pctx := newTestParsingContext(t, stackPath)

			for _, name := range tc.experiments {
				require.NoError(t, pctx.Experiments.EnableExperiment(name))
			}

			vars, err := config.TerragruntEvalVariables(ctx, logger.CreateLogger(), venv.MemVenv(), pctx, stackPath)
			require.NoError(t, err)

			assert.Equal(t, tc.want, vars)
		})
	}
}

func TestPathVariableUnitOfDependencyConfig(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	l := logger.CreateLogger()
	consumerPath := filepath.Join(root, "live", "app", config.DefaultTerragruntConfigPath)
	dependencyDir := filepath.Join(root, "live", "db")
	dependencyPath := filepath.Join(dependencyDir, config.DefaultTerragruntConfigPath)

	ctx, pctx := newPathParsingContext(t, consumerPath)

	l, pctx, err := pctx.WithDependencyConfigPath(l, dependencyPath)
	require.NoError(t, err)

	cfg, err := config.ParseConfigFile(ctx, l, v, pctx, dependencyPath, nil)
	require.NoError(t, err)

	assert.Equal(t, dependencyDir, cfg.Inputs["unit"])
	assert.Equal(t, dependencyDir, cfg.Inputs["env_unit"])
}

func TestPathVariableRequiresExperiment(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	cfgPath := filepath.Join(root, "values", "app", config.DefaultTerragruntConfigPath)

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.OriginalTerragruntConfigPath = cfgPath

	_, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
}

// pathFixture mirrors the path-variable fixture into memory and returns the venv and the fixture root.
func pathFixture(t *testing.T) (*venv.Venv, string) {
	t.Helper()

	fsys, root := venvtest.LoadFS(t, filepath.Join("..", "..", "test", "fixtures", "path-variable"))

	return venv.MemVenv().WithFS(fsys), root
}

// newPathParsingContext returns a parsing context for the unit at cfgPath with the path-variable experiment enabled.
func newPathParsingContext(t *testing.T, cfgPath string) (context.Context, *config.ParsingContext) {
	t.Helper()

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.OriginalTerragruntConfigPath = cfgPath
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.PathVariable))

	return ctx, pctx
}

// unitPathParsingContext returns the parsing context of the unit in unitDir with its `path` value set.
func unitPathParsingContext(t *testing.T, v *venv.Venv, unitDir string) (context.Context, *config.ParsingContext) {
	t.Helper()

	cfgPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	ctx, pctx := newPathParsingContext(t, cfgPath)
	l := logger.CreateLogger()

	file, err := pctx.NewParser(l, v).ParseFromFile(v.FS, cfgPath)
	require.NoError(t, err)

	pctx.Path, err = config.PathVariable(ctx, l, v, pctx, file, nil)
	require.NoError(t, err)
	require.NotNil(t, pctx.Path)

	return ctx, pctx
}

// generatePathStack generates the stack in stackDir with the named experiments enabled.
func generatePathStack(t *testing.T, v *venv.Venv, stackDir string, experiments ...string) error {
	t.Helper()

	stackPath := filepath.Join(stackDir, config.DefaultStackFile)
	ctx, pctx := newTestParsingContext(t, stackPath)
	pctx.TerragruntStackConfigPath = stackPath
	pctx.NoCAS = true

	for _, name := range experiments {
		require.NoError(t, pctx.Experiments.EnableExperiment(name))
	}

	pool := worker.NewWorkerPool(1)
	pool.Start()

	defer pool.Stop()

	if err := config.GenerateStackFile(ctx, logger.CreateLogger(), v, pctx, pool, stackPath); err != nil {
		return err
	}

	return pool.Wait()
}

// parsePathInputs parses the unit at cfgPath with the path-variable experiment enabled and returns its inputs.
func parsePathInputs(ctx context.Context, t *testing.T, v *venv.Venv, cfgPath string) map[string]any {
	t.Helper()

	_, pctx := newPathParsingContext(t, cfgPath)

	cfg, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)
	require.NoError(t, err)

	return cfg.Inputs
}

// parsePathUnit parses the unit in unitDir under its own caches and returns its inputs.
func parsePathUnit(t *testing.T, v *venv.Venv, unitDir string) map[string]any {
	t.Helper()

	cfgPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)

	return parsePathInputs(config.WithCaches(t.Context()), t, v, cfgPath)
}
