package config_test

import (
	"context"
	"encoding/json"
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
	"github.com/hashicorp/hcl/v2"
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
  path   = "app/${each.key}"
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
  path   = "team/${count.index}"
}
`

// unitWithExpansionJSON is the JSON encoding of unitWithExpansionHCL. Stack files are only
// ever HCL, but an include block may point at a JSON file, so the gate has to read one.
const unitWithExpansionJSON = `{
  "unit": {
    "app": {
      "expansion": {"for_each": ["web"]},
      "source": "./modules/app",
      "path": "app/${each.value}"
    }
  }
}`

// TestValidateBlockIterationExperimentGatesExpansion pins which block types reject an
// expansion block while the experiment is off, and that the error names the offending block.
func TestValidateBlockIterationExperimentGatesExpansion(t *testing.T) {
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
			name:          "unit with expansion in a json body",
			configPath:    "extra.json",
			cfg:           unitWithExpansionJSON,
			wantBlockType: "unit",
			wantLabel:     "app",
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

			err := config.ValidateBlockIterationExperiment(experiment.NewExperiments(), file)

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

// TestValidateBlockIterationExperimentGateClearsWhenOn pins that turning the experiment on
// clears the gate for both expansion blocks and bare enabled attributes.
func TestValidateBlockIterationExperimentGateClearsWhenOn(t *testing.T) {
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
		{
			name:       "unit with enabled",
			configPath: config.DefaultStackFile,
			cfg:        unitWithEnabledHCL,
		},
		{
			name:       "stack with enabled",
			configPath: config.DefaultStackFile,
			cfg:        stackWithEnabledHCL,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			experiments := experiment.NewExperiments()
			require.NoError(t, experiments.EnableExperiment(experiment.BlockIteration))

			require.NoError(
				t,
				config.ValidateBlockIterationExperiment(
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
	ctx, pctx := newTestParsingContext(t, venvtest.NewWithOSFS(), config.DefaultTerragruntConfigPath)

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
			ctx, pctx := newTestParsingContext(t, venvtest.NewWithOSFS(), config.DefaultStackFile)

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
	ctx, pctx := newTestParsingContext(t, venvtest.NewWithOSFS(), stackPath)
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

// TestDisabledExpandedDependencySkipsOutputRetrieval points every instance at a directory
// that does not exist, so the parse only succeeds if none of them went looking for outputs.
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

			dir := filepath.Join("/virtual", "include-merge")
			configPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)

			ctx, pctx := newExpansionParsingContext(t, configPath)
			fsys := pctx.Venv.FS

			require.NoError(t, vfs.EnsureDirectory(fsys, filepath.Join(dir, "network")))
			require.NoError(t, vfs.EnsureDirectory(fsys, filepath.Join(dir, "logging")))

			require.NoError(t, vfs.WriteFile(fsys, filepath.Join(dir, "root.hcl"), []byte(`
dependencies {
  paths = ["./network"]
}

dependency "vpc" {
  enabled     = false
  config_path = "../vpc"
}
`), 0o644))

			require.NoError(t, vfs.WriteFile(fsys, configPath, []byte(`
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
// mock outputs.
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

func TestUnitAndStackExpandPerIterationElement(t *testing.T) {
	t.Parallel()

	stackCfg, err := parseStackString(t, unitWithExpansionHCL+stackWithExpansionHCL)
	require.NoError(t, err)
	require.Len(t, stackCfg.Units, 2)
	require.Len(t, stackCfg.Stacks, 2)

	unitPaths := map[string]string{}

	for _, unit := range stackCfg.Units {
		require.NotNil(t, unit.Expansion)
		unitPaths[unit.Expansion.Key()] = unit.Path
	}

	assert.Equal(t, map[string]string{"web": "app/web", "api": "app/api"}, unitPaths)

	stackPaths := map[string]string{}

	for _, stack := range stackCfg.Stacks {
		require.NotNil(t, stack.Expansion)
		stackPaths[stack.Expansion.Key()] = stack.Path
	}

	assert.Equal(t, map[string]string{"0": "team/0", "1": "team/1"}, stackPaths)
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

// TestDependencyOutputsAddressedByInstanceKey pins the address an expanded dependency answers to,
// alongside an unexpanded block in the same config to hold its unkeyed address steady.
func TestDependencyOutputsAddressedByInstanceKey(t *testing.T) {
	t.Parallel()

	ctx, pctx := newExpansionParsingContext(t, config.DefaultTerragruntConfigPath)

	cfg, err := config.ParseConfigString(
		ctx,
		pctx,
		logger.CreateLogger(),
		config.DefaultTerragruntConfigPath,
		`
dependency "vpc" {
  config_path  = "../vpc"
  skip_outputs = true

  mock_outputs = {
    id = "vpc-main"
  }
}

dependency "aurora" {
  expansion {
    for_each = toset(["web", "api"])
  }

  config_path  = "../aurora-${each.key}"
  skip_outputs = true

  mock_outputs = {
    id = "aurora-${each.key}"
  }
}

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

inputs = {
  vpc_id      = dependency.vpc.outputs.id
  vpc_outputs = dependency.vpc.outputs
  web         = dependency.aurora["web"].outputs.id
  api         = dependency.aurora["api"].outputs.id
  first       = dependency.shard["0"].outputs.id
  second      = dependency.shard["1"].outputs.id
  unquoted    = dependency.shard[0].outputs.id
}
`,
		nil,
	)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"vpc_id":      "vpc-main",
		"vpc_outputs": map[string]any{"id": "vpc-main"},
		"web":         "aurora-web",
		"api":         "aurora-api",
		"first":       "shard-0",
		"second":      "shard-1",
		"unquoted":    "shard-0",
	}, cfg.Inputs)
}

// TestDependencyOutputsRejectExpandedBlockWithoutKey holds the keyed level to being the only
// address for an expanded block, since a config that reached past it would be reading an
// arbitrary instance.
func TestDependencyOutputsRejectExpandedBlockWithoutKey(t *testing.T) {
	t.Parallel()

	ctx, pctx := newExpansionParsingContext(t, config.DefaultTerragruntConfigPath)

	_, err := config.ParseConfigString(
		ctx,
		pctx,
		logger.CreateLogger(),
		config.DefaultTerragruntConfigPath,
		`
dependency "aurora" {
  expansion {
    for_each = toset(["web"])
  }

  config_path  = "../aurora-${each.key}"
  skip_outputs = true

  mock_outputs = {
    id = "aurora-${each.key}"
  }
}

inputs = {
  id = dependency.aurora.outputs.id
}
`,
		nil,
	)

	var diags hcl.Diagnostics
	require.ErrorAs(t, err, &diags)
	require.Len(t, diags, 1)
	// Naming the diagnostic keeps the test from passing on any other evaluation failure the
	// fixture might grow.
	assert.Equal(t, "Unsupported attribute", diags[0].Summary)
}

// TestDependencyOutputsEncodeDivergentSchemasPerKey covers instances whose outputs share no
// schema, so encoding cannot lean on the keys agreeing on a type.
func TestDependencyOutputsEncodeDivergentSchemasPerKey(t *testing.T) {
	t.Parallel()

	ctx, pctx := newExpansionParsingContext(t, config.DefaultTerragruntConfigPath)

	cfg, err := config.ParseConfigString(
		ctx,
		pctx,
		logger.CreateLogger(),
		config.DefaultTerragruntConfigPath,
		`
dependency "mixed" {
  expansion {
    for_each = {
      a = { id = "mixed-a" }
      b = { name = "mixed-b", size = 3 }
    }
  }

  config_path  = "../mixed-${each.key}"
  skip_outputs = true

  mock_outputs = each.value
}

inputs = {
  a_id   = dependency.mixed["a"].outputs.id
  b_name = dependency.mixed["b"].outputs.name
  b_size = dependency.mixed["b"].outputs.size
}
`,
		nil,
	)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"a_id":   "mixed-a",
		"b_name": "mixed-b",
		"b_size": json.Number("3"),
	}, cfg.Inputs)
}

// TestDependencyInstanceKeysMatchAddressableKeys ties the key the parser reports for an instance
// to the key a config writes to reach it.
func TestDependencyInstanceKeysMatchAddressableKeys(t *testing.T) {
	t.Parallel()

	ctx, pctx := newExpansionParsingContext(t, config.DefaultTerragruntConfigPath)

	cfg, err := config.ParseConfigString(
		ctx,
		pctx,
		logger.CreateLogger(),
		config.DefaultTerragruntConfigPath,
		`
dependency "numbered" {
  expansion {
    for_each = toset([1, 2])
  }

  config_path  = "../numbered-${each.key}"
  skip_outputs = true

  mock_outputs = {
    engine_key = each.key
  }
}

dependency "shard" {
  expansion {
    count = 2
  }

  config_path  = "../shard-${count.index}"
  skip_outputs = true

  mock_outputs = {
    engine_key = tostring(count.index)
  }
}

inputs = {
  numbered = {
    "1" = dependency.numbered["1"].outputs.engine_key
    "2" = dependency.numbered["2"].outputs.engine_key
  }

  shard = {
    "0" = dependency.shard["0"].outputs.engine_key
    "1" = dependency.shard["1"].outputs.engine_key
  }
}
`,
		nil,
	)
	require.NoError(t, err)

	addressed := map[string]map[string]any{}

	for _, name := range []string{"numbered", "shard"} {
		byKey, ok := cfg.Inputs[name].(map[string]any)
		require.True(t, ok)

		addressed[name] = byKey
	}

	reported := map[string][]string{}

	for _, dep := range cfg.TerragruntDependencies {
		require.NotNil(t, dep.Expansion)

		key := dep.Expansion.Key()
		reported[dep.Name] = append(reported[dep.Name], key)

		// The instance recorded the key the expansion engine handed its body, so an address
		// built from Key() only lands here if both agree on the stringification.
		assert.Equal(t, key, addressed[dep.Name][key])
	}

	assert.Equal(t, map[string][]string{
		"numbered": {"1", "2"},
		"shard":    {"0", "1"},
	}, reported)
}

// TestDependencyOutputsAddressEmptyEachKey covers an each.key that is itself the empty string,
// which is a key like any other and must not read as an absent one.
func TestDependencyOutputsAddressEmptyEachKey(t *testing.T) {
	t.Parallel()

	ctx, pctx := newExpansionParsingContext(t, config.DefaultTerragruntConfigPath)

	cfg, err := config.ParseConfigString(
		ctx,
		pctx,
		logger.CreateLogger(),
		config.DefaultTerragruntConfigPath,
		`
dependency "mixed" {
  expansion {
    for_each = toset(["", "web"])
  }

  config_path  = "../mixed-${each.key}"
  skip_outputs = true

  mock_outputs = {
    id = "mixed-${each.key}"
  }
}

inputs = {
  blank = dependency.mixed[""].outputs.id
  web   = dependency.mixed["web"].outputs.id
}
`,
		nil,
	)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"blank": "mixed-",
		"web":   "mixed-web",
	}, cfg.Inputs)
}

// TestDependencyLabelClaimedByBlockAndInstances covers a label that has to address both a whole
// block and a set of instances, which include merging is free to produce.
func TestDependencyLabelClaimedByBlockAndInstances(t *testing.T) {
	t.Parallel()

	ctx, pctx := newExpansionParsingContext(t, config.DefaultTerragruntConfigPath)

	_, err := config.ParseConfigString(
		ctx,
		pctx,
		logger.CreateLogger(),
		config.DefaultTerragruntConfigPath,
		`
dependency "aurora" {
  config_path  = "../aurora"
  skip_outputs = true

  mock_outputs = {
    id = "aurora-main"
  }
}

dependency "aurora" {
  expansion {
    for_each = toset(["web"])
  }

  config_path  = "../aurora-${each.key}"
  skip_outputs = true

  mock_outputs = {
    id = "aurora-${each.key}"
  }
}

inputs = {
  id = dependency.aurora["web"].outputs.id
}
`,
		nil,
	)

	var collision config.DependencyLabelCollisionError
	require.ErrorAs(t, err, &collision)
	assert.Equal(t, "aurora", collision.Name)
}

// TestDependencyOutputsResolveUnknownWhenOutputsSkipped covers hcl validate, where the placeholder
// standing in for unresolved outputs has to sit below the keyed level to be reachable.
func TestDependencyOutputsResolveUnknownWhenOutputsSkipped(t *testing.T) {
	t.Parallel()

	ctx, pctx := newExpansionParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx.SkipOutput = true
	pctx.SkipOutputsResolution = true

	_, err := config.ParseConfigString(
		ctx,
		pctx,
		logger.CreateLogger(),
		config.DefaultTerragruntConfigPath,
		`
dependency "aurora" {
  expansion {
    for_each = toset(["web", "api"])
  }

  config_path = "../does-not-exist-${each.key}"
}

inputs = {
  web = dependency.aurora["web"].outputs.id
  api = dependency.aurora["api"].outputs.id
}
`,
		nil,
	)
	require.NoError(t, err)
}

// TestDependencyInstancesAccumulateWithRacing drives enough instances through the concurrent
// output resolution to give the race detector something to catch.
func TestDependencyInstancesAccumulateWithRacing(t *testing.T) {
	t.Parallel()

	ctx, pctx := newExpansionParsingContext(t, config.DefaultTerragruntConfigPath)

	cfg, err := config.ParseConfigString(
		ctx,
		pctx,
		logger.CreateLogger(),
		config.DefaultTerragruntConfigPath,
		`
dependency "vpc" {
  config_path  = "../vpc"
  skip_outputs = true

  mock_outputs = {
    id = "vpc-main"
  }
}

dependency "shard" {
  expansion {
    count = 40
  }

  config_path  = "../shard-${count.index}"
  skip_outputs = true

  mock_outputs = {
    id = "shard-${count.index}"
  }
}

dependency "region" {
  expansion {
    for_each = toset([for i in range(40) : "r${i}"])
  }

  config_path  = "../region-${each.key}"
  skip_outputs = true

  mock_outputs = {
    id = "region-${each.key}"
  }
}

inputs = {
  vpc          = dependency.vpc.outputs.id
  first_shard  = dependency.shard["0"].outputs.id
  last_shard   = dependency.shard["39"].outputs.id
  first_region = dependency.region["r0"].outputs.id
  last_region  = dependency.region["r39"].outputs.id
}
`,
		nil,
	)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"vpc":          "vpc-main",
		"first_shard":  "shard-0",
		"last_shard":   "shard-39",
		"first_region": "region-r0",
		"last_region":  "region-r39",
	}, cfg.Inputs)
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

	ctx, pctx := newTestParsingContext(tb, venvtest.New(), configPath)
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
