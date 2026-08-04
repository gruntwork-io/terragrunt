package config_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

const dependencyWithExpansionHCL = `
dependency "aurora" {
  expansion {
    for_each = toset(["web", "api"])
  }

  config_path = "../aurora"
}
`

const unitWithExpansionHCL = `
unit "app" {
  expansion {
    for_each = toset(["web", "api"])
  }

  source = "./modules/app"
  path   = "app"
}
`

const jsonConfigPath = "terragrunt.hcl.json"

const jsonDependencyWithExpansion = `
{"dependency": {"aurora": {"expansion": {"count": 2}, "config_path": "../aurora-${count.index}"}}}
`

const stackWithExpansionHCL = `
stack "team" {
  expansion {
    count = 2
  }

  source = "./stacks/team"
  path   = "team"
}
`

// TestValidateExpansionExperiment pins which blocks the gate rejects while the
// block-iteration experiment is off, and that it names the offending block.
func TestValidateExpansionExperiment(t *testing.T) {
	t.Parallel()

	skipInExperimentMode(t)

	testCases := []struct {
		name          string
		configPath    string
		cfg           string
		wantBlockType string
		wantLabel     string
		wantErr       bool
	}{
		{
			name:          "dependency with expansion",
			configPath:    config.DefaultTerragruntConfigPath,
			cfg:           dependencyWithExpansionHCL,
			wantBlockType: "dependency",
			wantLabel:     "aurora",
			wantErr:       true,
		},
		{
			name:          "unit with expansion",
			configPath:    config.DefaultStackFile,
			cfg:           unitWithExpansionHCL,
			wantBlockType: "unit",
			wantLabel:     "app",
			wantErr:       true,
		},
		{
			name:          "stack with expansion",
			configPath:    config.DefaultStackFile,
			cfg:           stackWithExpansionHCL,
			wantBlockType: "stack",
			wantLabel:     "team",
			wantErr:       true,
		},
		{
			name:       "dependency without expansion",
			configPath: config.DefaultTerragruntConfigPath,
			cfg: `
dependency "vpc" {
  config_path = "../vpc"
}
`,
		},
		{
			name:       "unit without expansion",
			configPath: config.DefaultStackFile,
			cfg: `
unit "app" {
  source = "./modules/app"
  path   = "app"
}
`,
		},
		{
			name:       "expansion outside an expandable block",
			configPath: config.DefaultTerragruntConfigPath,
			cfg: `
generate "backend" {
  expansion {
    count = 2
  }

  path      = "backend.tf"
  if_exists = "overwrite"
  contents  = ""
}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file := parseHCLString(t, tc.cfg, tc.configPath)

			err := config.ValidateExpansionExperiment(experiment.NewExperiments(), file)

			if !tc.wantErr {
				require.NoError(t, err)
				return
			}

			var typed config.ExpansionRequiresExperimentError
			require.ErrorAs(t, err, &typed)
			assert.Equal(t, tc.wantBlockType, typed.BlockType)
			assert.Equal(t, tc.wantLabel, typed.BlockLabel)
			assert.Equal(t, tc.configPath, typed.ConfigPath)
		})
	}
}

// TestValidateExpansionExperimentEnabled pins that enabling the experiment clears the
// gate for every block type it covers.
func TestValidateExpansionExperimentEnabled(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		configPath string
		cfg        string
	}{
		{
			name:       "dependency",
			configPath: config.DefaultTerragruntConfigPath,
			cfg:        dependencyWithExpansionHCL,
		},
		{
			name:       "unit",
			configPath: config.DefaultStackFile,
			cfg:        unitWithExpansionHCL,
		},
		{
			name:       "stack",
			configPath: config.DefaultStackFile,
			cfg:        stackWithExpansionHCL,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			experiments := experiment.NewExperiments()
			require.NoError(t, experiments.EnableExperiment(experiment.BlockIteration))

			require.NoError(
				t,
				config.ValidateExpansionExperiment(
					experiments,
					parseHCLString(t, tc.cfg, tc.configPath),
				),
			)
		})
	}
}

// TestParseConfigStringExpansionRequiresExperiment proves the gate is wired into the
// unit config parse, not just callable on its own.
func TestParseConfigStringExpansionRequiresExperiment(t *testing.T) {
	t.Parallel()

	skipInExperimentMode(t)

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), config.DefaultTerragruntConfigPath)

	_, err := config.ParseConfigString(
		ctx,
		pctx,
		l,
		config.DefaultTerragruntConfigPath,
		dependencyWithExpansionHCL,
		nil,
	)

	var typed config.ExpansionRequiresExperimentError
	require.ErrorAs(t, err, &typed)
	assert.Equal(t, "dependency", typed.BlockType)
}

// TestReadStackConfigStringExpansionRequiresExperiment proves the gate is wired into the
// stack parse. A unit block decodes through an `hcl:",remain"` field that would otherwise
// absorb the expansion block without complaint.
func TestReadStackConfigStringExpansionRequiresExperiment(t *testing.T) {
	t.Parallel()

	skipInExperimentMode(t)

	testCases := []struct {
		name          string
		cfg           string
		wantBlockType string
	}{
		{
			name:          "unit",
			cfg:           unitWithExpansionHCL,
			wantBlockType: "unit",
		},
		{
			name:          "stack",
			cfg:           stackWithExpansionHCL,
			wantBlockType: "stack",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), config.DefaultStackFile)

			_, err := config.ReadStackConfigString(
				ctx,
				l,
				pctx,
				config.DefaultStackFile,
				tc.cfg,
				nil,
			)

			var typed config.ExpansionRequiresExperimentError
			require.ErrorAs(t, err, &typed)
			assert.Equal(t, tc.wantBlockType, typed.BlockType)
		})
	}
}

// TestReadStackConfigFileExpansionInIncludeRequiresExperiment pins that the gate follows
// include blocks. Included stack files decode straight to StackConfigFile without going
// back through ParseStackConfig, so they are gated separately.
func TestReadStackConfigFileExpansionInIncludeRequiresExperiment(t *testing.T) {
	t.Parallel()

	skipInExperimentMode(t)

	const dir = "/stack"

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(
		fsys,
		filepath.Join(dir, "included.stack.hcl"),
		[]byte(unitWithExpansionHCL),
		0o644,
	))

	stackPath := filepath.Join(dir, config.DefaultStackFile)
	require.NoError(t, vfs.WriteFile(fsys, stackPath, []byte(`
include "extra" {
  path = "included.stack.hcl"
}
`), 0o644))

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), stackPath)
	pctx.Venv.FS = fsys

	_, err := config.ReadStackConfigFile(ctx, l, pctx, stackPath, nil)

	var typed config.ExpansionRequiresExperimentError
	require.ErrorAs(t, err, &typed)
	assert.Equal(t, "unit", typed.BlockType)
}

// TestExpansionRequiresExperimentErrorNamesTheFlag pins that the error a user reads
// names the experiment they need, with and without a block label.
func TestExpansionRequiresExperimentErrorNamesTheFlag(t *testing.T) {
	t.Parallel()

	labeled := config.ExpansionRequiresExperimentError{
		ConfigPath: config.DefaultTerragruntConfigPath,
		BlockType:  "dependency",
		BlockLabel: "aurora",
	}
	assert.Contains(t, labeled.Error(), experiment.BlockIteration)
	assert.Contains(t, labeled.Error(), `dependency "aurora"`)

	unlabeled := config.ExpansionRequiresExperimentError{
		ConfigPath: config.DefaultStackFile,
		BlockType:  "unit",
	}
	assert.Contains(t, unlabeled.Error(), experiment.BlockIteration)
	assert.NotContains(t, unlabeled.Error(), `""`)
}

func TestDependencyExpandsForEach(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyString(t, `
dependency "aurora" {
  expansion {
    for_each = toset(["web", "api"])
  }

  config_path = "../${each.value}/aurora"
}
`)
	require.NoError(t, err)
	require.Len(t, cfg.TerragruntDependencies, 2)

	keys := make([]string, 0, len(cfg.TerragruntDependencies))
	paths := make([]string, 0, len(cfg.TerragruntDependencies))

	for _, dep := range cfg.TerragruntDependencies {
		require.NotNil(t, dep.Expansion)
		assert.Equal(t, "aurora", dep.Name)

		keys = append(keys, dep.Expansion.Key())
		paths = append(paths, dep.ConfigPath.AsString())
	}

	assert.ElementsMatch(t, []string{"web", "api"}, keys)
	assert.ElementsMatch(t, []string{"../web/aurora", "../api/aurora"}, paths)
}

func TestDependencyExpandsCount(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyString(t, `
dependency "shard" {
  expansion {
    count = 3
  }

  config_path = "../shard-${count.index}"
}
`)
	require.NoError(t, err)
	require.Len(t, cfg.TerragruntDependencies, 3)

	for index, dep := range cfg.TerragruntDependencies {
		require.NotNil(t, dep.Expansion)
		assert.Equal(t, strconv.Itoa(index), dep.Expansion.Key())
		assert.Equal(t, "../shard-"+strconv.Itoa(index), dep.ConfigPath.AsString())
	}
}

// TestDependencyExpansionResolvesPerInstance covers attributes other than config_path
// resolving separately per element.
func TestDependencyExpansionResolvesPerInstance(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyString(t, `
locals {
  regions = {
    use1 = true
    usw2 = false
  }
}

dependency "vpc" {
  expansion {
    for_each = local.regions
  }

  config_path = "../${each.key}/vpc"
  enabled     = each.value
}
`)
	require.NoError(t, err)
	require.Len(t, cfg.TerragruntDependencies, 2)

	enabled := map[string]bool{}

	for _, dep := range cfg.TerragruntDependencies {
		require.NotNil(t, dep.Enabled)
		enabled[dep.Expansion.Key()] = *dep.Enabled
	}

	assert.Equal(t, map[string]bool{"use1": true, "usw2": false}, enabled)
}

// TestDisabledExpandedDependencySkipsOutputRetrieval covers a whole disabled set: every
// instance points at a directory that does not exist, so the parse only succeeds if none
// of them went looking for outputs.
func TestDisabledExpandedDependencySkipsOutputRetrieval(t *testing.T) {
	t.Parallel()

	ctx, pctx := newExpansionParsingContext(t, config.DefaultTerragruntConfigPath)

	cfg, err := config.ParseConfigString(
		ctx,
		pctx,
		logger.CreateLogger(),
		config.DefaultTerragruntConfigPath,
		`
dependency "shard" {
  expansion {
    count = 2
  }

  enabled     = false
  config_path = "../no-such-unit-${count.index}"
}
`,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, cfg.TerragruntDependencies, 2)

	for _, dep := range cfg.TerragruntDependencies {
		require.NotNil(t, dep.Enabled)
		assert.False(t, *dep.Enabled)
		assert.Nil(t, dep.RenderedOutputs)
	}
}

// TestIncludeMergeKeepsEveryExpandedInstance covers an expanded set surviving an include
// merge, which matches dependencies up by label.
func TestIncludeMergeKeepsEveryExpandedInstance(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		mergeStrategy string
	}{
		{name: "shallow merge"},
		{name: "deep merge", mergeStrategy: `merge_strategy = "deep"`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			require.NoError(t, os.MkdirAll(filepath.Join(dir, "network"), 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "logging"), 0o755))

			require.NoError(t, os.WriteFile(filepath.Join(dir, "root.hcl"), []byte(`
dependencies {
  paths = ["./network"]
}

dependency "vpc" {
  enabled     = false
  config_path = "../vpc"
}
`), 0o644))

			configPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)
			require.NoError(t, os.WriteFile(configPath, []byte(`
include "root" {
  path = "root.hcl"
  `+tc.mergeStrategy+`
}

dependencies {
  paths = ["./logging"]
}

dependency "shard" {
  expansion {
    count = 3
  }

  enabled     = false
  config_path = "../shard-${count.index}"
}
`), 0o644))

			ctx, pctx := newExpansionParsingContext(t, configPath)

			cfg, err := config.ParseConfigFile(ctx, pctx, logger.CreateLogger(), configPath, nil)
			require.NoError(t, err)

			keys := make([]string, 0, len(cfg.TerragruntDependencies))

			for _, dep := range cfg.TerragruntDependencies {
				if dep.Name == "shard" {
					keys = append(keys, dep.Expansion.Key())
				}
			}

			assert.Equal(t, []string{"0", "1", "2"}, keys)
		})
	}
}

// TestExpandedDependencyCarriesItsOwnOutputConfig covers each instance resolving its own
// mock outputs. Retrieved outputs cannot be told apart per instance yet: the cty map they
// land in is still keyed by label alone.
func TestExpandedDependencyCarriesItsOwnOutputConfig(t *testing.T) {
	t.Parallel()

	ctx, pctx := newExpansionParsingContext(t, config.DefaultTerragruntConfigPath)

	cfg, err := config.ParseConfigString(
		ctx,
		pctx,
		logger.CreateLogger(),
		config.DefaultTerragruntConfigPath,
		`
dependency "shard" {
  expansion {
    count = 2
  }

  config_path  = "../shard-${count.index}"
  skip_outputs = true

  mock_outputs = {
    id = "shard-${count.index}"
  }
}
`,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, cfg.TerragruntDependencies, 2)

	outputs := map[string]string{}

	for _, dep := range cfg.TerragruntDependencies {
		require.NotNil(t, dep.MockOutputs)
		outputs[dep.Expansion.Key()] = dep.MockOutputs.GetAttr("id").AsString()
	}

	assert.Equal(t, map[string]string{"0": "shard-0", "1": "shard-1"}, outputs)
}

// TestJSONConfigDecodesDependencies covers the JSON syntax the expansion-aware decode has
// to keep serving.
func TestJSONConfigDecodesDependencies(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyJSONString(t, `
{"dependency": {"vpc": {"config_path": "../vpc", "enabled": false}}}
`)
	require.NoError(t, err)
	require.Len(t, cfg.TerragruntDependencies, 1)

	assert.Equal(t, "vpc", cfg.TerragruntDependencies[0].Name)
	assert.Equal(t, cty.StringVal("../vpc"), cfg.TerragruntDependencies[0].ConfigPath)
	assert.Nil(t, cfg.TerragruntDependencies[0].Expansion)
}

func TestJSONConfigExpandsDependencies(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyJSONString(t, jsonDependencyWithExpansion)
	require.NoError(t, err)
	require.Len(t, cfg.TerragruntDependencies, 2)

	paths := make([]string, 0, len(cfg.TerragruntDependencies))
	for _, dep := range cfg.TerragruntDependencies {
		paths = append(paths, dep.ConfigPath.AsString())
	}

	assert.Equal(t, []string{"../aurora-0", "../aurora-1"}, paths)
}

func TestJSONExpansionRequiresExperiment(t *testing.T) {
	t.Parallel()

	skipInExperimentMode(t)

	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), jsonConfigPath)

	_, err := config.PartialParseConfigString(
		ctx,
		pctx.WithDecodeList(config.DependencyBlock),
		logger.CreateLogger(),
		jsonConfigPath,
		jsonDependencyWithExpansion,
		nil,
	)

	var typed config.ExpansionRequiresExperimentError
	require.ErrorAs(t, err, &typed)
	assert.Equal(t, "dependency", typed.BlockType)
	assert.Equal(t, "aurora", typed.BlockLabel)
}

// TestUnknownBlockRemainsRejected guards against the header-only dependency decode
// loosening the strict schema into absorbing unrecognized blocks as remainder.
func TestUnknownBlockRemainsRejected(t *testing.T) {
	t.Parallel()

	ctx, pctx := newExpansionParsingContext(t, config.DefaultTerragruntConfigPath)

	_, err := config.ParseConfigString(
		ctx,
		pctx,
		logger.CreateLogger(),
		config.DefaultTerragruntConfigPath,
		`
bogus "x" {
  foo = 1
}
`,
		nil,
	)
	require.Error(t, err)
}

func TestUnitAndStackDecodeExpansionBlock(t *testing.T) {
	t.Parallel()

	stackCfg, err := parseStackString(t, unitWithExpansionHCL+stackWithExpansionHCL)
	require.NoError(t, err)
	require.Len(t, stackCfg.Units, 1)
	require.Len(t, stackCfg.Stacks, 1)

	unitExpansion := stackCfg.Units[0].Expansion
	require.NotNil(t, unitExpansion)
	require.NotNil(t, unitExpansion.ForEach)
	assert.Equal(
		t,
		cty.SetVal([]cty.Value{cty.StringVal("web"), cty.StringVal("api")}),
		*unitExpansion.ForEach,
	)

	stackExpansion := stackCfg.Stacks[0].Expansion
	require.NotNil(t, stackExpansion)
	require.NotNil(t, stackExpansion.Count)
	// Decoding and the literal build the number at different big.Float precisions,
	// which assert.Equal reads as a diff.
	assert.True(t, stackExpansion.Count.RawEquals(cty.NumberIntVal(2)))
}

func TestBlocksWithoutExpansionDecodeUnchanged(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyString(t, `
dependency "vpc" {
  config_path = "../vpc"
}
`)
	require.NoError(t, err)
	require.Len(t, cfg.TerragruntDependencies, 1)

	dep := cfg.TerragruntDependencies[0]
	assert.Nil(t, dep.Expansion)
	assert.Equal(t, cty.StringVal("../vpc"), dep.ConfigPath)

	stackCfg, err := parseStackString(t, `
unit "app" {
  source = "./modules/app"
  path   = "app"
}
`)
	require.NoError(t, err)
	require.Len(t, stackCfg.Units, 1)

	assert.Nil(t, stackCfg.Units[0].Expansion)
	assert.Equal(t, "app", stackCfg.Units[0].Path)
}

func TestExpansionBlockRejectsMistypedAttribute(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		parse func(testing.TB, string) error
		name  string
		cfg   string
	}{
		{
			name:  "dependency",
			parse: parseDependencyErr,
			cfg: `
dependency "aurora" {
  expansion {
    foreach = toset(["web"])
  }

  config_path = "../aurora"
}
`,
		},
		{
			name:  "unit",
			parse: parseStackErr,
			cfg: `
unit "app" {
  expansion {
    foreach = toset(["web"])
  }

  source = "./modules/app"
  path   = "app"
}
`,
		},
		{
			name:  "stack",
			parse: parseStackErr,
			cfg: `
stack "team" {
  expansion {
    cuont = 2
  }

  source = "./stacks/team"
  path   = "team"
}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Error(t, tc.parse(t, tc.cfg))
		})
	}
}

func TestIterationKeysAreNotSettableFromHCL(t *testing.T) {
	t.Parallel()

	_, err := parseDependencyString(t, `
dependency "aurora" {
  each_key = "web"

  config_path = "../aurora"
}
`)
	require.Error(t, err)

	stackCfg, err := parseStackString(t, `
unit "app" {
  each_key    = "web"
  count_index = 0

  source = "./modules/app"
  path   = "app"
}
`)
	require.NoError(t, err)
	require.Len(t, stackCfg.Units, 1)

	// A unit absorbs unknown attributes into its remainder rather than rejecting them,
	// so what it can be held to is that writing them leaves no expansion state.
	assert.Nil(t, stackCfg.Units[0].Expansion)
}

// TestDependencyCtyShapeExcludesExpansionMetadata pins the attribute set a dependency
// exposes to read_terragrunt_config and render, which a cty tag on expansion metadata
// would widen for every user, experiment or not.
func TestDependencyCtyShapeExcludesExpansionMetadata(t *testing.T) {
	t.Parallel()

	value, err := config.GoTypeToCty(config.Dependency{
		Name:       "vpc",
		ConfigPath: cty.StringVal("../vpc"),
	})
	require.NoError(t, err)

	attrs := make([]string, 0, len(value.Type().AttributeTypes()))
	for name := range value.Type().AttributeTypes() {
		attrs = append(attrs, name)
	}

	assert.ElementsMatch(t, []string{
		"name",
		"config_path",
		"enabled",
		"skip",
		"mock_outputs",
		"mock_outputs_allowed_terraform_commands",
		"mock_outputs_merge_with_state",
		"mock_outputs_merge_strategy_with_state",
		"outputs",
		"inputs",
	}, attrs)
}

func parseDependencyString(tb testing.TB, cfg string) (*config.TerragruntConfig, error) {
	tb.Helper()

	ctx, pctx := newExpansionParsingContext(tb, config.DefaultTerragruntConfigPath)

	return config.PartialParseConfigString(
		ctx,
		pctx.WithDecodeList(config.DependencyBlock),
		logger.CreateLogger(),
		config.DefaultTerragruntConfigPath,
		cfg,
		nil,
	)
}

func parseDependencyErr(tb testing.TB, cfg string) error {
	tb.Helper()

	_, err := parseDependencyString(tb, cfg)

	return err
}

func parseDependencyJSONString(
	tb testing.TB,
	cfg string,
) (*config.TerragruntConfig, error) {
	tb.Helper()

	ctx, pctx := newExpansionParsingContext(tb, jsonConfigPath)

	return config.PartialParseConfigString(
		ctx,
		pctx.WithDecodeList(config.DependencyBlock),
		logger.CreateLogger(),
		jsonConfigPath,
		cfg,
		nil,
	)
}

func parseStackString(tb testing.TB, cfg string) (*config.StackConfig, error) {
	tb.Helper()

	ctx, pctx := newExpansionParsingContext(tb, config.DefaultStackFile)

	return config.ReadStackConfigString(
		ctx,
		logger.CreateLogger(),
		pctx,
		config.DefaultStackFile,
		cfg,
		nil,
	)
}

func parseStackErr(tb testing.TB, cfg string) error {
	tb.Helper()

	_, err := parseStackString(tb, cfg)

	return err
}

func newExpansionParsingContext(
	tb testing.TB,
	configPath string,
) (context.Context, *config.ParsingContext) {
	tb.Helper()

	ctx, pctx := newTestParsingContext(tb, venvtest.NewOSWithEmptyEnv(), configPath)
	require.NoError(tb, pctx.Experiments.EnableExperiment(experiment.BlockIteration))

	return ctx, pctx
}

func parseHCLString(tb testing.TB, cfg, configPath string) *hclparse.File {
	tb.Helper()

	file, err := hclparse.NewParser().ParseFromString(cfg, configPath)
	require.NoError(tb, err)

	return file
}

// skipInExperimentMode skips a test that asserts disabled-experiment behavior, since
// TG_EXPERIMENT_MODE forces every experiment on.
func skipInExperimentMode(t *testing.T) {
	t.Helper()

	if helpers.IsExperimentMode(t) {
		t.Skip(
			"Skipping: TG_EXPERIMENT_MODE forces the block-iteration experiment on, so its disabled-state error can't be verified",
		)
	}
}
