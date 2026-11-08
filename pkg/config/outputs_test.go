package config_test

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// outputsBlockConfig declares two outputs with mock values and one without, with a mock strategy.
const outputsBlockConfig = `
outputs {
  mock_strategy {
    allowed_commands = ["validate", "plan"]
    merge_with_state = "shallow"
  }

  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "subnet_ids" {
    mock_value = ["subnet-mock-1", "subnet-mock-2"]
  }

  output "undocumented" {}
}
`

// dependentConfig reads two outputs of the vpc unit, with extra placed inside the dependency block.
const dependentConfig = `
dependency "vpc" {
  config_path = "../vpc"
  %s
}

inputs = {
  vpc_id     = dependency.vpc.outputs.vpc_id
  subnet_ids = dependency.vpc.outputs.subnet_ids
}
`

// dependentConfigWithoutInputs is dependentConfig without its inputs, for a test that reads the outputs its own way.
const dependentConfigWithoutInputs = `
dependency "vpc" {
  config_path = "../vpc"
  %s
}
`

// sensitivityInputs reads the two outputs of the vpc unit, with each one's sensitivity beside it.
const sensitivityInputs = `
inputs = {
  vpc_id               = nonsensitive(dependency.vpc.outputs.vpc_id)
  vpc_id_sensitive     = issensitive(dependency.vpc.outputs.vpc_id)
  subnet_ids           = dependency.vpc.outputs.subnet_ids
  subnet_ids_sensitive = issensitive(dependency.vpc.outputs.subnet_ids)
}
`

// vpcModule is the module of the vpc unit, which the output run of a dependency needs to find.
const vpcModule = `
output "vpc_id" {
  value = "vpc-real"
}
`

// outputsCacheDirMarker is the path element under which tofu runs for a dependency target.
const outputsCacheDirMarker = string(filepath.Separator) + ".terragrunt-cache" + string(filepath.Separator)

func TestParseOutputsBlock(t *testing.T) {
	t.Parallel()

	cfg := parseOutputsConfig(t, outputsBlockConfig, true)

	require.NotNil(t, cfg.Outputs)
	require.NotNil(t, cfg.Outputs.MockStrategy)
	require.NotNil(t, cfg.Outputs.MockStrategy.AllowedCommands)
	assert.Equal(t, []string{"validate", "plan"}, *cfg.Outputs.MockStrategy.AllowedCommands)
	require.NotNil(t, cfg.Outputs.MockStrategy.MergeWithState)
	assert.Equal(t, config.ShallowMerge, *cfg.Outputs.MockStrategy.MergeWithState)

	assert.Equal(t, []string{"vpc_id", "subnet_ids", "undocumented"}, cfg.Outputs.Names())
	assert.True(t, cfg.Outputs.Declares("vpc_id"))
	assert.False(t, cfg.Outputs.Declares("nat_id"))

	mocks := cfg.Outputs.MockValues()
	require.NotNil(t, mocks)
	assert.Equal(t, cty.StringVal("vpc-mock"), mocks.GetAttr("vpc_id"))
	assert.Equal(
		t,
		cty.TupleVal([]cty.Value{cty.StringVal("subnet-mock-1"), cty.StringVal("subnet-mock-2")}),
		mocks.GetAttr("subnet_ids"),
	)
	assert.False(t, mocks.Type().HasAttribute("undocumented"), "an output without mock_value has no mock")

	found, ok := cfg.GetFieldMetadata(config.MetadataOutputs)
	require.True(t, ok)
	assert.Equal(t, config.DefaultTerragruntConfigPath, found[config.FoundInFile])
}

func TestParseOutputsBlockRequiresExperiment(t *testing.T) {
	t.Parallel()

	l := createLogger()
	v := venvtest.New()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)

	_, err := config.ParseConfigString(ctx, l, v, pctx, config.DefaultTerragruntConfigPath, outputsBlockConfig, nil)

	var experimentErr config.OutputsBlockRequiresExperimentError

	require.ErrorAs(t, err, &experimentErr)
	assert.Equal(t, config.DefaultTerragruntConfigPath, experimentErr.ConfigPath)
}

func TestParseOutputsBlockRejectsMalformedBlocks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		check  func(t *testing.T, err error)
		name   string
		config string
	}{
		{
			name: "duplicate output",
			config: `
outputs {
  output "vpc_id" {}
  output "vpc_id" {}
}
`,
			check: func(t *testing.T, err error) {
				t.Helper()

				var duplicateErr config.DuplicateOutputDeclarationError

				require.ErrorAs(t, err, &duplicateErr)
				assert.Equal(t, "vpc_id", duplicateErr.Name)
			},
		},
		{
			name: "merge strategy that is not a mock merge strategy",
			config: `
outputs {
  mock_strategy {
    merge_with_state = "deep"
  }
}
`,
			check: func(t *testing.T, err error) {
				t.Helper()

				var strategyErr config.InvalidOutputsMergeStrategyError

				require.ErrorAs(t, err, &strategyErr)
				assert.Equal(t, "deep", strategyErr.Strategy)
			},
		},
		{
			name: "unknown attribute on an output",
			config: `
outputs {
  output "vpc_id" {
    type = string
  }
}
`,
			check: func(t *testing.T, err error) {
				t.Helper()

				requireDiagnostic(t, err, "Unsupported argument")
			},
		},
		{
			name: "output without a label",
			config: `
outputs {
  output {}
}
`,
			check: func(t *testing.T, err error) {
				t.Helper()

				requireDiagnostic(t, err, "Missing name for output")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := createLogger()
			v := venvtest.New()
			ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
			require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

			_, err := config.ParseConfigString(ctx, l, v, pctx, config.DefaultTerragruntConfigPath, tc.config, nil)
			require.Error(t, err)
			tc.check(t, err)
		})
	}
}

func TestOutputsConfigMerge(t *testing.T) {
	t.Parallel()

	vpcA := cty.StringVal("a")
	vpcB := cty.StringVal("b")
	arn := cty.StringVal("arn")

	target := &config.OutputsConfig{
		MockStrategy: &config.OutputsMockStrategy{AllowedCommands: new([]string{"plan"})},
		Outputs: []*config.OutputDeclaration{
			{Name: "vpc_id", MockValue: &vpcA},
			{Name: "subnet_ids"},
		},
	}
	source := &config.OutputsConfig{
		MockStrategy: &config.OutputsMockStrategy{MergeWithState: new(config.ShallowMerge)},
		Outputs: []*config.OutputDeclaration{
			{Name: "vpc_id", MockValue: &vpcB},
			{Name: "arn", MockValue: &arn},
		},
	}

	target.Merge(source)

	assert.Equal(t, []string{"plan"}, *target.MockStrategy.AllowedCommands, "an attribute the source leaves unset is kept")
	assert.Equal(t, config.ShallowMerge, *target.MockStrategy.MergeWithState)
	assert.Equal(t, []string{"vpc_id", "subnet_ids", "arn"}, target.Names())
	assert.Equal(t, vpcB, *target.Outputs[0].MockValue, "the source declaration wins")
	assert.Nil(t, target.Outputs[1].MockValue)
	assert.NotSame(t, source.Outputs[1], target.Outputs[2], "merged declarations are copies")
}

func TestOutputsBlockMergedThroughInclude(t *testing.T) {
	t.Parallel()

	const rootConfig = `
outputs {
  mock_strategy {
    allowed_commands = ["plan"]
  }

  output "vpc_id" {
    mock_value = "root-vpc"
  }

  output "arn" {
    mock_value = "root-arn"
  }
}
`

	const unitConfig = `
include "root" {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = "%s"
}

outputs {
  output "vpc_id" {
    mock_value = "unit-vpc"
  }
}
`

	testCases := []struct {
		name            string
		mergeStrategy   string
		expectedVPCMock string
		expectedNames   []string
		expectStrategy  bool
	}{
		{
			name:            "deep merge folds the root declarations in",
			mergeStrategy:   "deep",
			expectedNames:   []string{"vpc_id", "arn"},
			expectedVPCMock: "unit-vpc",
			expectStrategy:  true,
		},
		{
			name:            "shallow merge keeps the unit block",
			mergeStrategy:   "shallow",
			expectedNames:   []string{"vpc_id"},
			expectedVPCMock: "unit-vpc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			fsys := vfs.NewMemMapFS()
			require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "root.hcl"), []byte(rootConfig), 0o644))

			cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
			unitHCL := strings.ReplaceAll(unitConfig, "%s", tc.mergeStrategy)
			require.NoError(t, vfs.WriteFile(fsys, cfgPath, []byte(unitHCL), 0o644))

			l := createLogger()
			v := venvtest.New().WithFS(fsys)
			ctx, pctx := newTestParsingContext(t, cfgPath)
			require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

			cfg, err := config.ParseConfigFile(ctx, l, v, pctx, cfgPath, nil)
			require.NoError(t, err)
			require.NotNil(t, cfg.Outputs)

			assert.Equal(t, tc.expectedNames, cfg.Outputs.Names())
			assert.Equal(t, cty.StringVal(tc.expectedVPCMock), *cfg.Outputs.Outputs[0].MockValue)
			assert.Equal(t, tc.expectStrategy, cfg.Outputs.MockStrategy != nil)
		})
	}
}

func TestTerragruntConfigAsCtyRendersOutputs(t *testing.T) {
	t.Parallel()

	cfg := parseOutputsConfig(t, outputsBlockConfig, true)

	ctyVal, err := config.TerragruntConfigAsCty(cfg)
	require.NoError(t, err)

	rendered, err := ctyhelper.ParseCtyValueToMap(ctyVal)
	require.NoError(t, err)

	outputs, ok := rendered[config.MetadataOutputs].(map[string]any)
	require.True(t, ok, "outputs must render as an object")

	strategy, ok := outputs[config.MetadataMockStrategy].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "shallow", strategy["merge_with_state"])
	assert.Equal(t, []any{"validate", "plan"}, strategy["allowed_commands"])

	declarations, ok := outputs[config.MetadataOutput].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"mock_value": "vpc-mock"}, declarations["vpc_id"])
	assert.Equal(t, map[string]any{}, declarations["undocumented"])
}

func TestWriteToRendersOutputsBlock(t *testing.T) {
	t.Parallel()

	cfg := parseOutputsConfig(t, outputsBlockConfig, true)

	var rendered bytes.Buffer

	n, err := cfg.WriteTo(&rendered)
	require.NoError(t, err)
	assert.Positive(t, n)

	// The rendered block parses back to the same declarations.
	roundTrip := parseOutputsConfig(t, rendered.String(), true)
	assert.Equal(t, cfg.Outputs, roundTrip.Outputs)
}

func TestDependencyOutputReferences(t *testing.T) {
	t.Parallel()

	const content = `
locals {
  id = dependency.vpc.outputs.vpc_id
}

inputs = {
  subnets = dependency.vpc.outputs["subnet_ids"]
  db      = dependency.rds["primary"].outputs.endpoint
  replica = dependency.rds[1].outputs.endpoint
  dot     = dependency.rds.primary.outputs.endpoint
  nested  = [for id in [local.id] : dependency.vpc.outputs.vpc_id]
  looped  = [for dependency in [local.id] : dependency.vpc.outputs.shadow]
  whole   = dependency.vpc.outputs
  dynamic = dependency.vpc.outputs[local.key]
  named   = dependency.rds[local.name].outputs.endpoint
  other   = local.id
  index   = dependency["vpc"].outputs.vpc_id
  both    = dependency["rds"]["primary"]["outputs"]["endpoint"]
  marker  = dependency.vpc["outputs"].vpc_id
  self    = dependency.rds.outputs.outputs.endpoint
  stray   = dependency.vpc.extra.outputs.vpc_id
}

variables {
  inputs = {
    net = dependency.vpc.outputs.vpc_id
  }
}
`

	type reference struct {
		address string
		output  string
		line    int
	}

	l := createLogger()
	v := venvtest.New()
	_, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)

	file, err := pctx.NewParser(l, v).ParseFromString(content, config.DefaultTerragruntConfigPath)
	require.NoError(t, err)

	dependencies := config.Dependencies{
		{Name: "vpc"},
		{Name: "rds", Expansion: &hclparse.ExpansionBlock{}},
	}

	refs, err := config.DependencyOutputReferences(file, dependencies)
	require.NoError(t, err)

	found := make([]reference, 0, len(refs))

	for _, ref := range refs {
		found = append(found, reference{address: ref.Address(), output: ref.Output, line: ref.Range.Start.Line})
	}

	assert.Equal(t, []reference{
		{address: "vpc", output: "vpc_id", line: 3},
		{address: "vpc", output: "subnet_ids", line: 7},
		{address: "rds[primary]", output: "endpoint", line: 8},
		{address: "rds[1]", output: "endpoint", line: 9},
		{address: "rds[primary]", output: "endpoint", line: 10},
		{address: "vpc", output: "vpc_id", line: 11},
		{address: "vpc", output: "vpc_id", line: 17},
		{address: "rds[primary]", output: "endpoint", line: 18},
		{address: "vpc", output: "vpc_id", line: 19},
		{address: "rds[outputs]", output: "endpoint", line: 20},
		{address: "vpc", output: "vpc_id", line: 26},
	}, found)
}

func TestOutputDeclarationRanges(t *testing.T) {
	t.Parallel()

	l := createLogger()
	v := venvtest.New()
	_, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)

	file, err := pctx.NewParser(l, v).ParseFromString(outputsBlockConfig, config.DefaultTerragruntConfigPath)
	require.NoError(t, err)

	ranges := config.OutputDeclarationRanges(file)
	require.Len(t, ranges, 3)
	assert.Equal(t, 8, ranges["vpc_id"].Start.Line)
	assert.Equal(t, 12, ranges["subnet_ids"].Start.Line)
	assert.Equal(t, 16, ranges["undocumented"].Start.Line)
}

func TestDependencyReadsDeclaredOutputs(t *testing.T) {
	t.Parallel()

	const vpcStateWithAllOutputs = `{
  "vpc_id": {"sensitive": false, "type": "string", "value": "vpc-real"},
  "subnet_ids": {"sensitive": false, "type": ["list", "string"], "value": ["subnet-real"]},
  "undocumented": {"sensitive": false, "type": "string", "value": "real"}
}`

	const vpcStateWithoutSubnets = `{
  "vpc_id": {"sensitive": false, "type": "string", "value": "vpc-real"},
  "undocumented": {"sensitive": false, "type": "string", "value": "real"}
}`

	mockSubnets := []any{"subnet-mock-1", "subnet-mock-2"}

	testCases := []struct {
		check          func(t *testing.T, cfg *config.TerragruntConfig, err error)
		name           string
		vpcConfig      string
		dependencyBody string
		command        string
		vpcState       string
		inputs         string
		experimentOff  bool
	}{
		{
			name:      "declared mocks stand in while the unit has no state",
			vpcConfig: outputsBlockConfig,
			command:   "plan",
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-mock", cfg.Inputs["vpc_id"])
				assert.Equal(t, mockSubnets, cfg.Inputs["subnet_ids"])
			},
		},
		{
			name:           "the dependent's mock_outputs win key by key",
			vpcConfig:      outputsBlockConfig,
			dependencyBody: `mock_outputs = { vpc_id = "app-mock" }`,
			command:        "plan",
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "app-mock", cfg.Inputs["vpc_id"])
				assert.Equal(t, mockSubnets, cfg.Inputs["subnet_ids"])
			},
		},
		{
			name:      "a command outside allowed_commands reads no mocks",
			vpcConfig: outputsBlockConfig,
			command:   "apply",
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var noOutputsErr config.TerragruntOutputTargetNoOutputs

				require.ErrorAs(t, err, &noOutputsErr)
			},
		},
		{
			name:           "the dependent's allowed commands win over mock_strategy",
			vpcConfig:      outputsBlockConfig,
			dependencyBody: `mock_outputs_allowed_terraform_commands = ["apply"]`,
			command:        "apply",
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-mock", cfg.Inputs["vpc_id"])
			},
		},
		{
			name:      "state outputs win over declared mocks",
			vpcConfig: outputsBlockConfig,
			command:   "plan",
			vpcState:  vpcStateWithAllOutputs,
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-real", cfg.Inputs["vpc_id"])
				assert.Equal(t, []any{"subnet-real"}, cfg.Inputs["subnet_ids"])
			},
		},
		{
			name:      "a declared output missing from state is an error without a merge",
			vpcConfig: strings.ReplaceAll(outputsBlockConfig, `merge_with_state = "shallow"`, ""),
			command:   "plan",
			vpcState:  vpcStateWithoutSubnets,
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var missingErr config.DeclaredOutputMissingError

				require.ErrorAs(t, err, &missingErr)
				assert.Equal(t, "subnet_ids", missingErr.Output)
				assert.Equal(t, "vpc", missingErr.DependencyName)
				assert.True(t, strings.HasSuffix(missingErr.TargetConfig, filepath.Join("vpc", config.DefaultTerragruntConfigPath)))
			},
		},
		{
			name:      "a shallow merge fills a declared output missing from state",
			vpcConfig: outputsBlockConfig,
			command:   "plan",
			vpcState:  vpcStateWithoutSubnets,
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-real", cfg.Inputs["vpc_id"])
				assert.Equal(t, mockSubnets, cfg.Inputs["subnet_ids"])
			},
		},
		{
			name:      "a command outside allowed_commands does not merge a declared output missing from state",
			vpcConfig: outputsBlockConfig,
			command:   "apply",
			vpcState:  vpcStateWithoutSubnets,
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var missingErr config.DeclaredOutputMissingError

				require.ErrorAs(t, err, &missingErr)
				assert.Equal(t, "subnet_ids", missingErr.Output)
			},
		},
		{
			name:           "a dependent that skips outputs reads the declared mocks like mock_outputs",
			vpcConfig:      outputsBlockConfig,
			dependencyBody: `skip_outputs = true`,
			command:        "plan",
			vpcState:       vpcStateWithAllOutputs,
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-mock", cfg.Inputs["vpc_id"])
				assert.Equal(t, mockSubnets, cfg.Inputs["subnet_ids"])
			},
		},
		{
			name:           "a mock_outputs list replaces the declared mocks and cannot be read as an object",
			vpcConfig:      outputsBlockConfig,
			dependencyBody: `mock_outputs = ["oops"]`,
			command:        "plan",
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var diags hcl.Diagnostics

				require.ErrorAs(t, err, &diags)
			},
		},
		{
			name:           "a mock_outputs set of numbers replaces the declared mocks and cannot be read as an object",
			vpcConfig:      outputsBlockConfig,
			dependencyBody: `mock_outputs = toset([1])`,
			command:        "plan",
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var diags hcl.Diagnostics

				require.ErrorAs(t, err, &diags)
			},
		},
		{
			name:           "a mock_outputs list fills no declared output missing from state",
			vpcConfig:      outputsBlockConfig,
			dependencyBody: `mock_outputs = ["oops"]`,
			command:        "plan",
			vpcState:       vpcStateWithoutSubnets,
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var missingErr config.DeclaredOutputMissingError

				require.ErrorAs(t, err, &missingErr)
				assert.Equal(t, "subnet_ids", missingErr.Output)
			},
		},
		{
			name:           "a mock_outputs set of numbers fills no declared output missing from state",
			vpcConfig:      outputsBlockConfig,
			dependencyBody: `mock_outputs = toset([1])`,
			command:        "plan",
			vpcState:       vpcStateWithoutSubnets,
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var missingErr config.DeclaredOutputMissingError

				require.ErrorAs(t, err, &missingErr)
				assert.Equal(t, "subnet_ids", missingErr.Output)
			},
		},
		{
			name:           "a sensitive mock_outputs keeps its entries sensitive and the declared mocks plain",
			vpcConfig:      outputsBlockConfig,
			dependencyBody: `mock_outputs = sensitive({ vpc_id = "app-mock" })`,
			command:        "plan",
			inputs:         sensitivityInputs,
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "app-mock", cfg.Inputs["vpc_id"])
				assert.Equal(t, true, cfg.Inputs["vpc_id_sensitive"])
				assert.Equal(t, mockSubnets, cfg.Inputs["subnet_ids"])
				assert.Equal(t, false, cfg.Inputs["subnet_ids_sensitive"])
			},
		},
		{
			name:           "a sensitive mock_outputs fills a declared output missing from state",
			vpcConfig:      outputsBlockConfig,
			dependencyBody: `mock_outputs = sensitive({ subnet_ids = ["app-subnet"] })`,
			command:        "plan",
			vpcState:       vpcStateWithoutSubnets,
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-real", cfg.Inputs["vpc_id"])
				assert.Equal(t, []any{"app-subnet"}, cfg.Inputs["subnet_ids"])
			},
		},
		{
			name: "a mock value may read a local",
			vpcConfig: `
locals {
  mock = "local-mock"
}

outputs {
  output "vpc_id" {
    mock_value = local.mock
  }

  output "subnet_ids" {
    mock_value = []
  }
}
`,
			command: "plan",
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "local-mock", cfg.Inputs["vpc_id"])
			},
		},
		{
			name: "a mock value that reads a dependency output is an error",
			vpcConfig: `
dependency "net" {
  config_path = "../net"
}

outputs {
  output "vpc_id" {
    mock_value = dependency.net.outputs.cidr
  }
}
`,
			command: "plan",
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var unknownErr config.UnknownMockValueError

				require.ErrorAs(t, err, &unknownErr)
				assert.Equal(t, "vpc_id", unknownErr.Output)
				assert.True(t, strings.HasSuffix(unknownErr.ConfigPath, filepath.Join("vpc", config.DefaultTerragruntConfigPath)))

				var blockErr config.DependencyBlockError

				require.ErrorAs(t, err, &blockErr)
				assert.Equal(t, "vpc", blockErr.Name)
			},
		},
		{
			name: "a mock value that fails to evaluate is a read error with the diagnostic",
			vpcConfig: `
locals {
  mock = "local-mock"
}

outputs {
  output "vpc_id" {
    mock_value = local.mok
  }
}
`,
			command: "plan",
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var readErr config.DeclaredOutputsReadError

				require.ErrorAs(t, err, &readErr)
				assert.True(t, strings.HasSuffix(readErr.TargetConfig, filepath.Join("vpc", config.DefaultTerragruntConfigPath)))
				requireDiagnostic(t, readErr.Err, "Unsupported attribute")
			},
		},
		{
			name: "a dependency with a broken config is an error, not a unit without the block",
			vpcConfig: `
outputs {
  output "vpc_id" {
`,
			command: "plan",
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				requireDiagnostic(t, err, "Unclosed configuration block")
			},
		},
		{
			name:          "the experiment off rejects a unit that declares outputs",
			vpcConfig:     outputsBlockConfig,
			command:       "plan",
			experimentOff: true,
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var experimentErr config.OutputsBlockRequiresExperimentError

				require.ErrorAs(t, err, &experimentErr)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			fsys := vfs.NewMemMapFS()
			vpcDir := filepath.Join(root, "vpc")
			vpcCfgPath := filepath.Join(vpcDir, config.DefaultTerragruntConfigPath)
			require.NoError(t, vfs.WriteFile(fsys, vpcCfgPath, []byte(tc.vpcConfig), 0o644))
			require.NoError(t, vfs.WriteFile(fsys, filepath.Join(vpcDir, "main.tf"), []byte(vpcModule), 0o644))

			appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
			appConfig := strings.Replace(dependentConfig, "%s", tc.dependencyBody, 1)

			if tc.inputs != "" {
				appConfig = strings.Replace(dependentConfigWithoutInputs, "%s", tc.dependencyBody, 1) + tc.inputs
			}

			require.NoError(t, vfs.WriteFile(fsys, appConfigPath, []byte(appConfig), 0o644))

			state := map[string]string{}
			if tc.vpcState != "" {
				state[vpcDir] = tc.vpcState
			}

			l := createLogger()
			v := venvtest.New().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(state)))
			ctx, pctx := newDependentParsingContext(t, appConfigPath, tc.command)

			if !tc.experimentOff {
				require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))
			}

			cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
			tc.check(t, cfg, err)
		})
	}
}

func TestDependencyReadGivesOutputsBlockTip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		vpcConfig     string
		expectedTipAt []string
	}{
		{
			name:          "a unit without an outputs block gets the tip",
			vpcConfig:     ``,
			expectedTipAt: []string{filepath.Join("vpc", config.DefaultTerragruntConfigPath)},
		},
		{
			name:      "a unit with an outputs block gets no tip",
			vpcConfig: outputsBlockConfig,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			fsys := vfs.NewMemMapFS()
			vpcConfigPath := filepath.Join(root, "vpc", config.DefaultTerragruntConfigPath)
			require.NoError(t, vfs.WriteFile(fsys, vpcConfigPath, []byte(tc.vpcConfig), 0o644))
			require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "vpc", "main.tf"), []byte(vpcModule), 0o644))

			appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
			appConfig := strings.Replace(dependentConfig, "%s", `mock_outputs = { vpc_id = "m", subnet_ids = [] }`, 1)
			require.NoError(t, vfs.WriteFile(fsys, appConfigPath, []byte(appConfig), 0o644))

			l := createLogger()
			v := venvtest.New().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(nil)))
			ctx, pctx := newDependentParsingContext(t, appConfigPath, "plan")
			require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

			var tippedAt []string

			pctx.OutputsBlockTip = func(_ log.Logger, configPath string) {
				tippedAt = append(tippedAt, strings.TrimPrefix(configPath, root+string(filepath.Separator)))
			}

			_, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedTipAt, tippedAt)
		})
	}
}

// TestDependencyReadsDeclaredOutputsThroughInclude pins that the outputs block a unit merges from an included
// config reaches its dependents.
func TestDependencyReadsDeclaredOutputsThroughInclude(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "root.hcl"), []byte(outputsBlockConfig), 0o644))

	vpcConfig := `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`
	vpcDir := filepath.Join(root, "vpc")
	vpcConfigPath := filepath.Join(vpcDir, config.DefaultTerragruntConfigPath)
	require.NoError(t, vfs.WriteFile(fsys, vpcConfigPath, []byte(vpcConfig), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(vpcDir, "main.tf"), []byte(vpcModule), 0o644))

	appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
	appConfig := strings.Replace(dependentConfig, "%s", "", 1)
	require.NoError(t, vfs.WriteFile(fsys, appConfigPath, []byte(appConfig), 0o644))

	l := createLogger()
	v := venvtest.New().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(nil)))
	ctx, pctx := newDependentParsingContext(t, appConfigPath, "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "vpc-mock", cfg.Inputs["vpc_id"])
	assert.Equal(t, []any{"subnet-mock-1", "subnet-mock-2"}, cfg.Inputs["subnet_ids"])
}

// TestDependencyOnStackReadsNoDeclaredOutputs pins that a dependency whose config_path points at a stack keeps
// reading its own mock_outputs, with no outputs block read and no tip.
func TestDependencyOnStackReadsNoDeclaredOutputs(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	fsys := vfs.NewMemMapFS()
	stackConfigPath := filepath.Join(root, "stack", config.DefaultStackFile)
	stackConfig := `
unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}
`
	require.NoError(t, vfs.WriteFile(fsys, stackConfigPath, []byte(stackConfig), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "units", "vpc", "main.tf"), []byte(vpcModule), 0o644))
	require.NoError(t, vfs.WriteFile(
		fsys,
		filepath.Join(root, "units", "vpc", config.DefaultTerragruntConfigPath),
		[]byte(``),
		0o644,
	))

	appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
	appConfig := `
dependency "stack" {
  config_path = "../stack"

  mock_outputs = {
    vpc_id = "stack-mock"
  }
}

inputs = {
  vpc_id = dependency.stack.outputs.vpc_id
}
`
	require.NoError(t, vfs.WriteFile(fsys, appConfigPath, []byte(appConfig), 0o644))

	l := createLogger()
	v := venvtest.New().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(nil)))
	ctx, pctx := newDependentParsingContext(t, appConfigPath, "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	tipped := false

	pctx.OutputsBlockTip = func(log.Logger, string) {
		tipped = true
	}

	cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "stack-mock", cfg.Inputs["vpc_id"])
	assert.False(t, tipped, "a stack target gets no outputs-block tip")
}

// TestDependencyDeclaredOutputsReadsEachUnitOnce pins the per-run cache of declared outputs: a second read of
// the same unit config evaluates nothing again, and a changed file is read again. The unit's locals read a tfvars
// file, so each evaluation opens it once.
func TestDependencyDeclaredOutputsReadsEachUnitOnce(t *testing.T) {
	t.Parallel()

	const vpcConfig = `
locals {
  vars = jsondecode(read_tfvars_file("${get_terragrunt_dir()}/vpc.tfvars"))
}

outputs {
  output "vpc_id" {
    mock_value = local.vars.mock
  }
}
`

	root := venvtest.Root("/fixture")
	fsys := &openCountingFS{FS: vfs.NewMemMapFS(), opens: map[string]int{}}
	vpcDir := filepath.Join(root, "vpc")
	vpcConfigPath := filepath.Join(vpcDir, config.DefaultTerragruntConfigPath)
	tfvarsPath := filepath.Join(vpcDir, "vpc.tfvars")

	require.NoError(t, vfs.WriteFile(fsys, vpcConfigPath, []byte(vpcConfig), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, tfvarsPath, []byte(`mock = "from-tfvars"`), 0o644))

	l := createLogger()
	v := venvtest.New().WithFS(fsys)
	ctx, pctx := newDependentParsingContext(t, filepath.Join(root, "app", config.DefaultTerragruntConfigPath), "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	dep := config.Dependency{Name: "vpc", ConfigPath: cty.StringVal("../vpc")}

	first, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)

	second, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)

	require.NotNil(t, first)
	assert.Equal(t, "from-tfvars", first.Outputs[0].MockValue.AsString())
	assert.Equal(t, first.Names(), second.Names())
	assert.NotSame(t, first, second, "each read gets its own copy")
	assert.Equal(t, 1, fsys.count(tfvarsPath), "the second read came from the cache")

	require.NoError(t, vfs.WriteFile(fsys, vpcConfigPath, []byte(vpcConfig+"\n"), 0o644))

	third, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)
	assert.Equal(t, first.Names(), third.Names())
	assert.Equal(t, 2, fsys.count(tfvarsPath), "a changed unit config is read again")
}

// TestDependencyDeclaredOutputsConcurrentReadsWithRacing pins that dependents reading one target at once each get
// a copy and the target is parsed no more often than there are readers.
func TestDependencyDeclaredOutputsConcurrentReadsWithRacing(t *testing.T) {
	t.Parallel()

	const readers = 8

	root := venvtest.Root("/fixture")
	fsys := &openCountingFS{FS: vfs.NewMemMapFS(), opens: map[string]int{}}
	vpcConfigPath := filepath.Join(root, "vpc", config.DefaultTerragruntConfigPath)

	require.NoError(t, vfs.WriteFile(fsys, vpcConfigPath, []byte(outputsBlockConfig), 0o644))

	l := createLogger()
	v := venvtest.New().WithFS(fsys)
	ctx, pctx := newDependentParsingContext(t, filepath.Join(root, "app", config.DefaultTerragruntConfigPath), "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	dep := config.Dependency{Name: "vpc", ConfigPath: cty.StringVal("../vpc")}
	results := make([]*config.OutputsConfig, readers)
	errs := make([]error, readers)

	var wg sync.WaitGroup

	for i := range readers {
		wg.Go(func() {
			results[i], errs[i] = dep.DeclaredOutputs(ctx, l, v, pctx)
		})
	}

	wg.Wait()

	for i := range readers {
		require.NoError(t, errs[i])
		require.NotNil(t, results[i])
		assert.Equal(t, results[0].Names(), results[i].Names())
	}

	opens := fsys.count(vpcConfigPath)
	assert.GreaterOrEqual(t, opens, 1)
	assert.LessOrEqual(t, opens, readers)
}

// parseOutputsConfig parses content as a unit config, with the outputs-block experiment on when enabled.
func parseOutputsConfig(t *testing.T, content string, enabled bool) *config.TerragruntConfig {
	t.Helper()

	l := createLogger()
	v := venvtest.New()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)

	if enabled {
		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))
	}

	cfg, err := config.ParseConfigString(ctx, l, v, pctx, config.DefaultTerragruntConfigPath, content, nil)
	require.NoError(t, err)

	return cfg
}

// newDependentParsingContext returns a parsing context for cfgPath that runs command, so mock gating sees it.
func newDependentParsingContext(t *testing.T, cfgPath, command string) (context.Context, *config.ParsingContext) {
	t.Helper()

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.TerraformCommand = command
	pctx.OriginalTofuCommand = command
	pctx.TerraformCliArgs = iacargs.New().SetCommand(command)

	return ctx, pctx
}

// tofuOutputStub answers `tofu output -json` with the state of the unit whose cache directory it runs in, keyed
// by unit directory, and with no outputs for any other unit. Other commands succeed with no output.
func tofuOutputStub(state map[string]string) vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if len(inv.Args) == 0 || inv.Args[0] != "output" {
			return vexec.Result{}
		}

		unitDir, _, _ := strings.Cut(inv.Dir, outputsCacheDirMarker)
		if out, ok := state[unitDir]; ok {
			return vexec.Result{Stdout: []byte(out)}
		}

		return vexec.Result{Stdout: []byte("{}")}
	}
}

// requireDiagnostic requires that err carries HCL diagnostics, one of which has summary.
func requireDiagnostic(t *testing.T, err error, summary string) {
	t.Helper()

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	assert.True(t, slices.ContainsFunc(diags, func(diag *hcl.Diagnostic) bool {
		return diag.Summary == summary
	}), "no %q diagnostic in: %s", summary, diags.Error())
}

// openCountingFS counts how often each file is opened, by its cleaned path.
type openCountingFS struct {
	vfs.FS
	opens map[string]int
	mu    sync.Mutex
}

func (fsys *openCountingFS) Open(name string) (vfs.File, error) {
	fsys.mu.Lock()
	fsys.opens[filepath.Clean(name)]++
	fsys.mu.Unlock()

	return fsys.FS.Open(name)
}

// count returns how often name was opened.
func (fsys *openCountingFS) count(name string) int {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()

	return fsys.opens[filepath.Clean(name)]
}
