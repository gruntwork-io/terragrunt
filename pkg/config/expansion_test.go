package config_test

import (
	"context"
	"path/filepath"
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

func TestDependencyDecodesExpansionBlock(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyString(t, dependencyWithExpansionHCL)
	require.NoError(t, err)
	require.Len(t, cfg.TerragruntDependencies, 1)

	dep := cfg.TerragruntDependencies[0]
	require.NotNil(t, dep.Expansion)
	require.NotNil(t, dep.Expansion.ForEach)

	assert.Equal(
		t,
		cty.SetVal([]cty.Value{cty.StringVal("web"), cty.StringVal("api")}),
		*dep.Expansion.ForEach,
	)
	assert.Nil(t, dep.Expansion.Count)
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
