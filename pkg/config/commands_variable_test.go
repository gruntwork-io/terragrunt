package config_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestCommandsVariableInUnitIncludeAndRead(t *testing.T) {
	t.Parallel()

	v, root := commandsFixture(t)
	cfgPath := filepath.Join(root, "live", "app", config.DefaultTerragruntConfigPath)

	cfg := parseCommandsUnit(t, v, cfgPath)

	assert.Equal(t, map[string]any{
		"vars":                 anyList(config.TerraformCommandsNeedVars),
		"locking":              anyList(config.TerraformCommandsNeedLocking),
		"input":                anyList(config.TerraformCommandsNeedInput),
		"parallelism":          anyList(config.TerraformCommandsNeedParallelism),
		"indexed":              anyList(config.TerraformCommandsNeedVars),
		"function_vars":        anyList(config.TerraformCommandsNeedVars),
		"function_locking":     anyList(config.TerraformCommandsNeedLocking),
		"function_input":       anyList(config.TerraformCommandsNeedInput),
		"function_parallelism": anyList(config.TerraformCommandsNeedParallelism),
		"env_input":            anyList(config.TerraformCommandsNeedInput),
		"root_locking":         anyList(config.TerraformCommandsNeedLocking),
		"plan_accepts_vars":    true,
		"not_defined":          "not-defined",
	}, cfg.Inputs)
}

func TestCommandsVariableInHookAndExtraArguments(t *testing.T) {
	t.Parallel()

	v, root := commandsFixture(t)
	cfgPath := filepath.Join(root, "live", "app", config.DefaultTerragruntConfigPath)

	cfg := parseCommandsUnit(t, v, cfgPath)

	require.NotNil(t, cfg.Terraform)
	require.Len(t, cfg.Terraform.ExtraArgs, 1)
	require.Len(t, cfg.Terraform.BeforeHooks, 1)

	assert.Equal(t, config.TerraformCommandsNeedLocking, cfg.Terraform.ExtraArgs[0].Commands)
	assert.Equal(t, config.TerraformCommandsNeedVars, cfg.Terraform.BeforeHooks[0].Commands)
}

func TestCommandsVariableInJSONConfig(t *testing.T) {
	t.Parallel()

	v, root := commandsFixture(t)
	cfgPath := filepath.Join(root, "live", "json", config.DefaultTerragruntJSONConfigPath)

	cfg := parseCommandsUnit(t, v, cfgPath)

	assert.Equal(t, map[string]any{
		"vars":    anyList(config.TerraformCommandsNeedVars),
		"indexed": anyList(config.TerraformCommandsNeedParallelism),
	}, cfg.Inputs)
}

func TestCommandsVariableInDependencyBlock(t *testing.T) {
	t.Parallel()

	v, root := commandsFixture(t)
	cfgPath := filepath.Join(root, "dependency", "app", config.DefaultTerragruntConfigPath)

	cfg := parseCommandsUnit(t, v, cfgPath)

	assert.Equal(t, map[string]any{"db_input": anyList(config.TerraformCommandsNeedInput)}, cfg.Inputs)
}

func TestCommandsVariableInPartialParse(t *testing.T) {
	t.Parallel()

	v, root := commandsFixture(t)
	cfgPath := filepath.Join(root, "live", "app", config.DefaultTerragruntConfigPath)

	ctx, pctx := newCommandsParsingContext(t, cfgPath)
	pctx = pctx.WithDecodeList(config.TerraformBlock)

	cfg, err := new(config.Store).ResolvePartialConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)
	require.NoError(t, err)
	require.NotNil(t, cfg.Terraform)
	require.Len(t, cfg.Terraform.BeforeHooks, 1)

	assert.Equal(t, config.TerraformCommandsNeedVars, cfg.Terraform.BeforeHooks[0].Commands)
}

func TestCommandsVariableInValuesFile(t *testing.T) {
	t.Parallel()

	v, root := commandsFixture(t)
	cfgPath := filepath.Join(root, "values", "app", config.DefaultTerragruntConfigPath)

	cfg := parseCommandsUnit(t, v, cfgPath)

	assert.Equal(t, map[string]any{"from_values": anyList(config.TerraformCommandsNeedParallelism)}, cfg.Inputs)
}

func TestCommandsVariableAsWholeObject(t *testing.T) {
	t.Parallel()

	const unit = `
locals {
  names = keys(commands)
}

inputs = {
  names = local.names
  all   = commands
}
`

	root := venvtest.Root("/fixture")
	cfgPath := filepath.Join(root, config.DefaultTerragruntConfigPath)
	v := venv.MemVenv().WithFS(memFS(root, map[string]string{config.DefaultTerragruntConfigPath: unit})(t))

	cfg := parseCommandsUnit(t, v, cfgPath)

	assert.Equal(t, map[string]any{
		"names": []any{"accept_input", "accept_locking", "accept_parallelism", "accept_vars"},
		"all": map[string]any{
			"accept_vars":        anyList(config.TerraformCommandsNeedVars),
			"accept_locking":     anyList(config.TerraformCommandsNeedLocking),
			"accept_input":       anyList(config.TerraformCommandsNeedInput),
			"accept_parallelism": anyList(config.TerraformCommandsNeedParallelism),
		},
	}, cfg.Inputs)
}

func TestCommandsVariableInStackFile(t *testing.T) {
	t.Parallel()

	v, root := commandsFixture(t)
	stackPath := filepath.Join(root, "stack", "live", config.DefaultStackFile)

	ctx, pctx := newCommandsParsingContext(t, stackPath)

	stack, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), v, new(config.Store), pctx, stackPath)
	require.NoError(t, err)
	require.Len(t, stack.Units, 1)
	require.NotNil(t, stack.Units[0].Values)

	values, err := ctyhelper.ParseCtyValueToMap(*stack.Units[0].Values)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"vars":    anyList(config.TerraformCommandsNeedVars),
		"locking": anyList(config.TerraformCommandsNeedLocking),
		"input":   anyList(config.TerraformCommandsNeedInput),
	}, values)
}

func TestCommandsVariableInGeneratedStack(t *testing.T) {
	t.Parallel()

	v, root := commandsFixture(t)
	stackDir := filepath.Join(root, "stack", "live")
	cfgPath := filepath.Join(stackDir, config.StackDir, "app", config.DefaultTerragruntConfigPath)

	require.NoError(t, generateCommandsStack(t, v, stackDir, experiment.CommandsVariable))

	cfg := parseCommandsUnit(t, v, cfgPath)

	assert.Equal(t, map[string]any{
		"vars":        anyList(config.TerraformCommandsNeedVars),
		"locking":     anyList(config.TerraformCommandsNeedLocking),
		"input":       anyList(config.TerraformCommandsNeedInput),
		"parallelism": anyList(config.TerraformCommandsNeedParallelism),
	}, cfg.Inputs)
}

func TestCommandsVariableInAutoIncludeBlockIsGeneratedAsLiteral(t *testing.T) {
	t.Parallel()

	v, root := commandsFixture(t)
	stackDir := filepath.Join(root, "autoinclude", "live")
	cfgPath := filepath.Join(stackDir, config.StackDir, "app", config.DefaultTerragruntConfigPath)

	require.NoError(t, generateCommandsStack(t, v, stackDir, experiment.CommandsVariable))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.OriginalTerragruntConfigPath = cfgPath

	cfg, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"unit":      "app",
		"auto_vars": anyList(config.TerraformCommandsNeedVars),
	}, cfg.Inputs)
}

func TestCommandsVariableInAutoIncludeBlockRequiresExperiment(t *testing.T) {
	t.Parallel()

	v, root := commandsFixture(t)

	err := generateCommandsStack(t, v, filepath.Join(root, "autoinclude", "live"))

	var unresolved inthclparse.PartialEvalUnresolvedError

	require.ErrorAs(t, err, &unresolved)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	assert.True(t, namesCommandsExperiment(diags))
}

func TestCommandsVariableInAutoIncludeDependencyExpansion(t *testing.T) {
	t.Parallel()

	v, root := commandsFixture(t)
	stackDir := filepath.Join(root, "autoinclude-expansion", "live")
	cfgPath := filepath.Join(stackDir, config.StackDir, "app", config.DefaultTerragruntConfigPath)

	require.NoError(t, generateCommandsStack(t, v, stackDir, experiment.CommandsVariable, experiment.StackDependencies))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.OriginalTerragruntConfigPath = cfgPath

	cfg, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)
	require.NoError(t, err)

	assert.ElementsMatch(t, anyList(config.TerraformCommandsNeedParallelism), cfg.Inputs["db_commands"])
}

func TestCommandsVariableInAutoIncludeDependencyExpansionRequiresExperiment(t *testing.T) {
	t.Parallel()

	v, root := commandsFixture(t)
	stackDir := filepath.Join(root, "autoinclude-expansion", "live")

	err := generateCommandsStack(t, v, stackDir, experiment.StackDependencies)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	assert.True(t, namesCommandsExperiment(diags))
}

func TestCommandsVariableInStackLocalsNamesTheExperimentOnlyWhenDisabled(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		experiments []string
		hint        bool
	}{
		{name: "experiment disabled", hint: true},
		{name: "experiment enabled", experiments: []string{experiment.CommandsVariable}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, root := commandsFixture(t)
			stackPath := filepath.Join(root, "stack-locals", config.DefaultStackFile)

			ctx, pctx := newTestParsingContext(t, stackPath)

			for _, name := range tc.experiments {
				require.NoError(t, pctx.Experiments.EnableExperiment(name))
			}

			_, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), v, new(config.Store), pctx, stackPath)
			if !tc.hint {
				require.NoError(t, err)

				return
			}

			var diags hcl.Diagnostics

			require.ErrorAs(t, err, &diags)
			require.Len(t, diags, 1)
			require.NotNil(t, diags[0].Subject)

			assert.Equal(t, stackPath, diags[0].Subject.Filename)
			assert.True(t, namesCommandsExperiment(diags))
		})
	}
}

func TestCommandsVariableHintSkipsAnErrorTheExperimentDoesNotFix(t *testing.T) {
	t.Parallel()

	const unit = `
values {
  value "vars" {
    default = commands.accept_vars
  }
}
`

	root := venvtest.Root("/fixture")
	cfgPath := filepath.Join(root, config.DefaultTerragruntConfigPath)
	v := venv.MemVenv().WithFS(memFS(root, map[string]string{config.DefaultTerragruntConfigPath: unit})(t))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.OriginalTerragruntConfigPath = cfgPath
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.ValuesBlock))

	_, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	require.NotEmpty(t, diags)
	assert.False(t, namesCommandsExperiment(diags))
}

func TestCommandsVariableErrorNamesTheExperimentWhenDisabled(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		config string
	}{
		{name: "locals", config: filepath.Join("disabled", "locals", config.DefaultTerragruntConfigPath)},
		{name: "inputs", config: filepath.Join("disabled", "inputs", config.DefaultTerragruntConfigPath)},
		{name: "values file", config: filepath.Join("disabled", "values", config.DefaultTerragruntConfigPath)},
		{name: "JSON config", config: filepath.Join("live", "json", config.DefaultTerragruntJSONConfigPath)},
		{name: "included file", config: filepath.Join("live", "app", config.DefaultTerragruntConfigPath)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, root := commandsFixture(t)
			cfgPath := filepath.Join(root, tc.config)

			ctx, pctx := newTestParsingContext(t, cfgPath)
			pctx.OriginalTerragruntConfigPath = cfgPath

			_, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)

			var diags hcl.Diagnostics

			require.ErrorAs(t, err, &diags)
			assert.True(t, namesCommandsExperiment(diags))
		})
	}
}

func TestCommandsVariableHintSkipsAnIteratorNamedCommands(t *testing.T) {
	t.Parallel()

	const unit = `
locals {
  upper = [for commands in ["plan"] : upper(commands)]
}

inputs = {
  upper   = local.upper
  missing = [for commands in [{}] : commands.missing]
}
`

	root := venvtest.Root("/fixture")
	cfgPath := filepath.Join(root, config.DefaultTerragruntConfigPath)
	v := venv.MemVenv().WithFS(memFS(root, map[string]string{config.DefaultTerragruntConfigPath: unit})(t))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.OriginalTerragruntConfigPath = cfgPath

	_, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	require.NotEmpty(t, diags)
	assert.False(t, namesCommandsExperiment(diags))
}

func commandsFixture(t *testing.T) (*venv.Venv, string) {
	t.Helper()

	fsys, root := venvtest.LoadFS(t, filepath.Join("..", "..", "test", "fixtures", "commands-variable"))

	return venv.MemVenv().WithFS(fsys), root
}

func newCommandsParsingContext(t *testing.T, cfgPath string) (context.Context, *config.ParsingContext) {
	t.Helper()

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.OriginalTerragruntConfigPath = cfgPath
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.CommandsVariable))

	return ctx, pctx
}

func parseCommandsUnit(t *testing.T, v *venv.Venv, cfgPath string) *config.TerragruntConfig {
	t.Helper()

	ctx, pctx := newCommandsParsingContext(t, cfgPath)

	cfg, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)
	require.NoError(t, err)

	return cfg
}

func generateCommandsStack(t *testing.T, v *venv.Venv, stackDir string, experiments ...string) error {
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

	if err := config.GenerateStackFile(
		ctx,
		logger.CreateLogger(),
		v,
		new(config.Store),
		pctx,
		pool,
		stackPath,
		func(config.TryDefault) {},
	); err != nil {
		return err
	}

	return pool.Wait()
}

func namesCommandsExperiment(diags hcl.Diagnostics) bool {
	return slices.ContainsFunc(diags, func(diag *hcl.Diagnostic) bool {
		return strings.Count(diag.Detail, "--experiment "+experiment.CommandsVariable) == 1
	})
}

func anyList(items []string) []any {
	list := make([]any, 0, len(items))
	for _, item := range items {
		list = append(list, item)
	}

	return list
}
