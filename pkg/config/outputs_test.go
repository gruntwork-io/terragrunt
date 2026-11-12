package config_test

import (
	"bytes"
	"cmp"
	"context"
	"io"
	"net/http"
	"net/url"
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
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

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

const dependentConfigWithoutInputs = `
dependency "vpc" {
  config_path = "../vpc"
  %s
}
`

const singleOutputDependentConfig = `
dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
`

const sensitivityInputs = `
inputs = {
  vpc_id               = nonsensitive(dependency.vpc.outputs.vpc_id)
  vpc_id_sensitive     = issensitive(dependency.vpc.outputs.vpc_id)
  subnet_ids           = dependency.vpc.outputs.subnet_ids
  subnet_ids_sensitive = issensitive(dependency.vpc.outputs.subnet_ids)
}
`

const nestedSensitivityInputs = `
inputs = {
  token_id               = dependency.vpc.outputs.token.id
  token_id_sensitive     = issensitive(dependency.vpc.outputs.token.id)
  token_secret           = dependency.vpc.outputs.token.secret
  token_secret_sensitive = issensitive(dependency.vpc.outputs.token.secret)
}
`

const vpcModule = `
output "vpc_id" {
  value = "vpc-real"
}
`

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
	v := venv.MemVenv()
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
			v := venv.MemVenv()
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
			v := venv.MemVenv().WithFS(fsys)
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
	v := venv.MemVenv()
	_, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)

	file, err := pctx.NewParser(l, v).ParseFromString(content, config.DefaultTerragruntConfigPath)
	require.NoError(t, err)

	dependencies := config.Dependencies{
		{Name: "vpc"},
		{Name: "rds", Expansion: &hclparse.ExpansionBlock{}},
	}

	refs, err := config.DependencyOutputReferences(file, dependencies, nil)
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

func TestDependencyOutputReferencesBoundsBlockNesting(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		depth    int
		exceeded bool
	}{
		{name: "blocks nested to the limit are walked", depth: 32},
		{name: "blocks nested past the limit are an error", depth: 33, exceeded: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			content := strings.Repeat("a {\n", tc.depth) +
				"id = dependency.vpc.outputs.vpc_id\n" +
				strings.Repeat("}\n", tc.depth)

			l := createLogger()
			v := venv.MemVenv()
			_, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)

			file, err := pctx.NewParser(l, v).ParseFromString(content, config.DefaultTerragruntConfigPath)
			require.NoError(t, err)

			refs, err := config.DependencyOutputReferences(file, config.Dependencies{{Name: "vpc"}}, nil)

			if !tc.exceeded {
				require.NoError(t, err)
				require.Len(t, refs, 1)
				assert.Equal(t, "vpc_id", refs[0].Output)

				return
			}

			var nestingErr config.BlockNestingExceededError

			require.ErrorAs(t, err, &nestingErr)
			assert.Equal(t, 32, nestingErr.Limit)
			assert.Equal(t, config.DefaultTerragruntConfigPath, nestingErr.ConfigPath)
		})
	}
}

func TestDependencyOutputReferencesSkipsReplacedBlocks(t *testing.T) {
	t.Parallel()

	const content = `
dependency "db" {
  config_path  = "../db"
  mock_outputs = { id = dependency.vpc.outputs.skipped }
}

dependency "db" {
  config_path  = "../db"
  mock_outputs = { id = dependency.vpc.outputs.expanded }

  expansion {
    for_each = toset(["a"])
  }
}

dependency "cache" {
  config_path  = "../cache"
  mock_outputs = { id = dependency.vpc.outputs.kept }
}

generate "nested" {
  dependency "db" {
    mock_outputs = { id = dependency.vpc.outputs.nested }
  }
}
`

	l := createLogger()
	v := venv.MemVenv()
	_, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)

	file, err := pctx.NewParser(l, v).ParseFromString(content, config.DefaultTerragruntConfigPath)
	require.NoError(t, err)

	replaced := map[string]struct{}{"db": {}}

	refs, err := config.DependencyOutputReferences(file, config.Dependencies{{Name: "vpc"}}, replaced)
	require.NoError(t, err)

	outputs := make([]string, 0, len(refs))
	for _, ref := range refs {
		outputs = append(outputs, ref.Output)
	}

	assert.Equal(t, []string{"expanded", "kept", "nested"}, outputs)
}

func TestOutputDeclarationRanges(t *testing.T) {
	t.Parallel()

	l := createLogger()
	v := venv.MemVenv()
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

	const vpcStateWithOnlyUndocumented = `{
  "undocumented": {"sensitive": false, "type": "string", "value": "real"}
}`

	const vpcStateWithoutSubnets = `{
  "vpc_id": {"sensitive": false, "type": "string", "value": "vpc-real"},
  "undocumented": {"sensitive": false, "type": "string", "value": "real"}
}`

	const vpcStateWithToken = `{
  "vpc_id": {"sensitive": false, "type": "string", "value": "vpc-real"},
  "token": {"sensitive": false, "type": ["object", {"id": "string"}], "value": {"id": "token-real"}},
  "undocumented": {"sensitive": false, "type": "string", "value": "real"}
}`

	const tokenDeclaration = `
  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "token" {
    mock_value = {
      id     = "token-mock"
      secret = sensitive("token-secret")
    }
  }
}
`

	mockSubnets := []any{"subnet-mock-1", "subnet-mock-2"}

	testCases := []struct {
		check             func(t *testing.T, cfg *config.TerragruntConfig, err error)
		name              string
		vpcConfig         string
		vpcConfigFile     string
		vpcAutoInclude    string
		dependencyBody    string
		command           string
		vpcState          string
		inputs            string
		experimentOff     bool
		ignoreDiagnostics bool
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
				assert.True(t, strings.HasSuffix(missingErr.TargetConfig, filepath.Join("vpc", config.DefaultTerragruntConfigPath)))

				var blockErr config.DependencyBlockError

				require.ErrorAs(t, err, &blockErr)
				assert.Equal(t, "vpc", blockErr.Name)
			},
		},
		{
			name:      "every declared output missing from state is reported",
			vpcConfig: strings.ReplaceAll(outputsBlockConfig, `merge_with_state = "shallow"`, ""),
			command:   "plan",
			vpcState:  vpcStateWithOnlyUndocumented,
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var blockErr config.DependencyBlockError

				require.ErrorAs(t, err, &blockErr)

				var joined interface{ Unwrap() []error }

				require.ErrorAs(t, blockErr.Err, &joined)

				missing := make([]string, 0, len(joined.Unwrap()))

				for _, leaf := range joined.Unwrap() {
					var missingErr config.DeclaredOutputMissingError

					require.ErrorAs(t, leaf, &missingErr)

					missing = append(missing, missingErr.Output)
				}

				assert.ElementsMatch(t, []string{"vpc_id", "subnet_ids"}, missing)
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
			name: "a mock value that fails to evaluate is an error with the diagnostic",
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

				requireDiagnostic(t, err, "Unsupported attribute")
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
			name: "a disabled dependency reads no outputs block",
			vpcConfig: `
outputs {
  output "vpc_id" {
`,
			dependencyBody: `enabled = false`,
			command:        "plan",
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var diags hcl.Diagnostics

				require.ErrorAs(t, err, &diags)
				assert.NotErrorAs(t, err, new(config.DependencyBlockError))
			},
		},
		{
			name: "a sensitive declared mock value stays sensitive for the dependent",
			vpcConfig: `
outputs {
  output "vpc_id" {
    mock_value = sensitive("vpc-secret")
  }

  output "subnet_ids" {
    mock_value = []
  }
}
`,
			command: "plan",
			inputs:  sensitivityInputs,
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-secret", cfg.Inputs["vpc_id"])
				assert.Equal(t, true, cfg.Inputs["vpc_id_sensitive"])
				assert.Equal(t, false, cfg.Inputs["subnet_ids_sensitive"])
			},
		},
		{
			name: "a sensitive declared mock value filled into state keeps its mark",
			vpcConfig: `
outputs {
  mock_strategy {
    merge_with_state = "shallow"
  }

  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "subnet_ids" {
    mock_value = sensitive(["subnet-secret"])
  }
}
`,
			command:  "plan",
			vpcState: vpcStateWithoutSubnets,
			inputs:   sensitivityInputs,
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-real", cfg.Inputs["vpc_id"])
				assert.Equal(t, false, cfg.Inputs["vpc_id_sensitive"])
				assert.Equal(t, []any{"subnet-secret"}, cfg.Inputs["subnet_ids"])
				assert.Equal(t, true, cfg.Inputs["subnet_ids_sensitive"])
			},
		},
		{
			name: "a sensitive nested declared mock field filled into state keeps its mark",
			vpcConfig: `
outputs {
  mock_strategy {
    merge_with_state = "deep_map_only"
  }
` + tokenDeclaration,
			command:  "plan",
			vpcState: vpcStateWithToken,
			inputs:   nestedSensitivityInputs,
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "token-real", cfg.Inputs["token_id"])
				assert.Equal(t, false, cfg.Inputs["token_id_sensitive"])
				assert.Equal(t, "token-secret", cfg.Inputs["token_secret"])
				assert.Equal(t, true, cfg.Inputs["token_secret_sensitive"])
			},
		},
		{
			name: "a shallow merge keeps an output of the state whole",
			vpcConfig: `
outputs {
  mock_strategy {
    merge_with_state = "shallow"
  }
` + tokenDeclaration,
			command:  "plan",
			vpcState: vpcStateWithToken,
			inputs:   nestedSensitivityInputs,
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				requireDiagnostic(t, err, "Unsupported attribute")
			},
		},
		{
			name: "a reader that ignores diagnostics reads no mocks from a broken unit config",
			vpcConfig: `
outputs {
  output "vpc_id" {
`,
			command:           "plan",
			ignoreDiagnostics: true,
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var noOutputsErr config.TerragruntOutputTargetNoOutputs

				require.ErrorAs(t, err, &noOutputsErr)
			},
		},
		{
			name:           "the deprecated mock_outputs_merge_with_state wins over merge_with_state",
			vpcConfig:      outputsBlockConfig,
			dependencyBody: `mock_outputs_merge_with_state = false`,
			command:        "plan",
			vpcState:       vpcStateWithoutSubnets,
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var missingErr config.DeclaredOutputMissingError

				require.ErrorAs(t, err, &missingErr)
				assert.Equal(t, "subnet_ids", missingErr.Output)

				var blockErr config.DependencyBlockError

				require.ErrorAs(t, err, &blockErr)
				assert.Equal(t, "vpc", blockErr.Name)
			},
		},
		{
			name:          "a unit declared in terragrunt.hcl.json is read",
			vpcConfigFile: "terragrunt.hcl.json",
			vpcConfig: `{
  "outputs": {
    "output": {
      "vpc_id": {"mock_value": "vpc-json"},
      "subnet_ids": {"mock_value": []}
    }
  }
}`,
			command: "plan",
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-json", cfg.Inputs["vpc_id"])
			},
		},
		{
			name:           "an outputs block in the autoinclude file of the unit is read",
			vpcAutoInclude: outputsBlockConfig,
			command:        "plan",
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-mock", cfg.Inputs["vpc_id"])
				assert.Equal(t, mockSubnets, cfg.Inputs["subnet_ids"])
			},
		},
		{
			name:      "an instance of an expanded dependency reads the declared mocks",
			vpcConfig: outputsBlockConfig,
			dependencyBody: `expansion {
    for_each = toset(["vpc"])
  }`,
			command: "plan",
			inputs: `
inputs = {
  vpc_id     = dependency.vpc["vpc"].outputs.vpc_id
  subnet_ids = dependency.vpc["vpc"].outputs.subnet_ids
}
`,
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-mock", cfg.Inputs["vpc_id"])
				assert.Equal(t, mockSubnets, cfg.Inputs["subnet_ids"])
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

				var blockErr config.DependencyBlockError

				require.ErrorAs(t, err, &blockErr)
				assert.Equal(t, "vpc", blockErr.Name)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			fsys := vfs.NewMemMapFS()
			vpcDir := filepath.Join(root, "vpc")
			vpcCfgPath := filepath.Join(vpcDir, cmp.Or(tc.vpcConfigFile, config.DefaultTerragruntConfigPath))
			require.NoError(t, vfs.WriteFile(fsys, vpcCfgPath, []byte(tc.vpcConfig), 0o644))
			require.NoError(t, vfs.WriteFile(fsys, filepath.Join(vpcDir, "main.tf"), []byte(vpcModule), 0o644))

			if tc.vpcAutoInclude != "" {
				autoIncludePath := filepath.Join(vpcDir, "terragrunt.autoinclude.hcl")
				require.NoError(t, vfs.WriteFile(fsys, autoIncludePath, []byte(tc.vpcAutoInclude), 0o644))
			}

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
			v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(state)))
			ctx, pctx := newDependentParsingContext(t, appConfigPath, tc.command)

			if !tc.experimentOff {
				require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))
			}

			pctx.Parser.IgnoreDiagnostics = tc.ignoreDiagnostics

			cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
			tc.check(t, cfg, err)
		})
	}
}

func TestUnitsWithoutOutputsNamesTheUnitsDependenciesRead(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		vpcConfig   string
		experiments []string
		expected    []string
	}{
		{
			name:        "a unit without an outputs block is named",
			vpcConfig:   ``,
			experiments: []string{experiment.OutputsBlock},
			expected:    []string{filepath.Join("vpc", config.DefaultTerragruntConfigPath)},
		},
		{
			name:        "a unit with an outputs block is not named",
			vpcConfig:   outputsBlockConfig,
			experiments: []string{experiment.OutputsBlock},
			expected:    []string{},
		},
		{
			name:      "without the experiment no unit is named",
			vpcConfig: ``,
			expected:  []string{},
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
			v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(nil)))
			ctx, pctx := newDependentParsingContext(t, appConfigPath, "plan")

			for _, name := range tc.experiments {
				require.NoError(t, pctx.Experiments.EnableExperiment(name))
			}

			cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
			require.NoError(t, err)

			targets, err := config.UnitsWithoutOutputs(ctx, l, v, pctx, cfg)
			require.NoError(t, err)

			named := make([]string, 0, len(targets))

			for _, target := range targets {
				named = append(named, strings.TrimPrefix(target, root+string(filepath.Separator)))
			}

			assert.Equal(t, tc.expected, named)
		})
	}
}

// TestDependencyReadsDeclaredOutputsThroughInclude expects an outputs block merged from an include to reach dependents.
func TestDependencyReadsDeclaredOutputsThroughInclude(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		vpcConfig     string
		expectedVPCID string
	}{
		{
			name: "the block of an include alone",
			vpcConfig: `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`,
			expectedVPCID: "vpc-mock",
		},
		{
			name: "a deep merge with the block of the unit",
			vpcConfig: `
include "root" {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = "deep"
}

outputs {
  output "vpc_id" {
    mock_value = "unit-mock"
  }
}
`,
			expectedVPCID: "unit-mock",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			fsys := vfs.NewMemMapFS()
			vpcDir := filepath.Join(root, "vpc")
			appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
			files := map[string]string{
				filepath.Join(root, "root.hcl"):                           outputsBlockConfig,
				filepath.Join(vpcDir, config.DefaultTerragruntConfigPath): tc.vpcConfig,
				filepath.Join(vpcDir, "main.tf"):                          vpcModule,
				appConfigPath:                                             strings.Replace(dependentConfig, "%s", "", 1),
			}

			for path, content := range files {
				require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
			}

			l := createLogger()
			v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(nil)))
			ctx, pctx := newDependentParsingContext(t, appConfigPath, "plan")
			require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

			cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedVPCID, cfg.Inputs["vpc_id"])
			assert.Equal(t, []any{"subnet-mock-1", "subnet-mock-2"}, cfg.Inputs["subnet_ids"])
		})
	}
}

// TestIncludedDependencyReadsDeclaredOutputs expects a dependency block of an included file to read declared mocks.
func TestIncludedDependencyReadsDeclaredOutputs(t *testing.T) {
	t.Parallel()

	const rootConfig = `
dependency "vpc" {
  config_path = "${get_terragrunt_dir()}/../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
`

	const appConfig = `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`

	root := venvtest.Root("/fixture")
	fsys := vfs.NewMemMapFS()
	vpcDir := filepath.Join(root, "vpc")
	appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
	files := map[string]string{
		filepath.Join(root, "root.hcl"):                           rootConfig,
		filepath.Join(vpcDir, config.DefaultTerragruntConfigPath): outputsBlockConfig,
		filepath.Join(vpcDir, "main.tf"):                          vpcModule,
		appConfigPath:                                             appConfig,
	}

	for path, content := range files {
		require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
	}

	l := createLogger()
	v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(nil)))
	ctx, pctx := newDependentParsingContext(t, appConfigPath, "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "vpc-mock", cfg.Inputs["vpc_id"])
}

// TestDependencyOnStackReadsNoDeclaredOutputs expects a dependency on a stack to keep its own mock_outputs and no tip.
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
	v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(nil)))
	ctx, pctx := newDependentParsingContext(t, appConfigPath, "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "stack-mock", cfg.Inputs["vpc_id"])

	targets, err := config.UnitsWithoutOutputs(ctx, l, v, pctx, cfg)
	require.NoError(t, err)
	assert.Empty(t, targets, "a stack target is not a unit without outputs")
}

// TestDependencyOnStackDirectoryWithUnitFileReadsNoDeclaredOutputs expects a stack target to keep its unit file out.
func TestDependencyOnStackDirectoryWithUnitFileReadsNoDeclaredOutputs(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	fsys := vfs.NewMemMapFS()
	vpcDir := filepath.Join(root, "vpc")
	appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
	appConfig := strings.Replace(dependentConfigWithoutInputs, "%s", `mock_outputs = { vpc_id = "app-mock" }`, 1) + `
inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
`
	stackConfig := `
unit "network" {
  source = "../units/network"
  path   = "network"
}
`
	files := map[string]string{
		filepath.Join(vpcDir, config.DefaultStackFile):                              stackConfig,
		filepath.Join(vpcDir, config.DefaultTerragruntConfigPath):                   outputsBlockConfig,
		filepath.Join(root, "units", "network", "main.tf"):                          vpcModule,
		filepath.Join(root, "units", "network", config.DefaultTerragruntConfigPath): "",
		appConfigPath: appConfig,
	}

	for path, content := range files {
		require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
	}

	l := createLogger()
	v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(nil)))
	ctx, pctx := newDependentParsingContext(t, appConfigPath, "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
	require.NoError(t, err)
	require.Len(t, cfg.TerragruntDependencies, 1)

	declared, err := cfg.TerragruntDependencies[0].DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)
	assert.Nil(t, declared)

	targets, err := config.UnitsWithoutOutputs(ctx, l, v, pctx, cfg)
	require.NoError(t, err)
	assert.Empty(t, targets)
}

// TestDependentParseRecordsTheFilesDeclaredOutputsRead expects a file a mock value reads among the dependent's files.
func TestDependentParseRecordsTheFilesDeclaredOutputsRead(t *testing.T) {
	t.Parallel()

	const vpcConfig = `
outputs {
  output "vpc_id" {
    mock_value = jsondecode(read_tfvars_file("vpc.tfvars")).mock
  }
}
`

	root := venvtest.Root("/fixture")
	fsys := vfs.NewMemMapFS()
	vpcDir := filepath.Join(root, "vpc")
	tfvarsPath := filepath.Join(vpcDir, "vpc.tfvars")
	appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
	files := map[string]string{
		filepath.Join(vpcDir, config.DefaultTerragruntConfigPath): vpcConfig,
		filepath.Join(vpcDir, "main.tf"):                          vpcModule,
		tfvarsPath:                                                `mock = "from-tfvars"`,
		appConfigPath:                                             singleOutputDependentConfig,
	}

	for path, content := range files {
		require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
	}

	l := createLogger()
	v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(nil)))
	ctx, pctx := newDependentParsingContext(t, appConfigPath, "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	pctx.FilesRead = config.NewFilesRead()

	cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "from-tfvars", cfg.Inputs["vpc_id"])
	assert.Contains(t, pctx.FilesRead.Paths(), tfvarsPath)
}

// TestDependentsReadDeclaredOutputsUnderTheirOwnEnvironment expects no dependent to get the value of another.
func TestDependentsReadDeclaredOutputsUnderTheirOwnEnvironment(t *testing.T) {
	t.Parallel()

	const vpcConfig = `
outputs {
  output "vpc_id" {
    mock_value = get_env("MOCK_VPC_ID", "unset")
  }
}
`

	root := venvtest.Root("/fixture")
	fsys := vfs.NewMemMapFS()
	vpcDir := filepath.Join(root, "vpc")

	vpcConfigPath := filepath.Join(vpcDir, config.DefaultTerragruntConfigPath)

	require.NoError(t, vfs.WriteFile(fsys, vpcConfigPath, []byte(vpcConfig), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(vpcDir, "main.tf"), []byte(vpcModule), 0o644))

	l := createLogger()
	ctx := config.WithCaches(t.Context())

	for _, dependent := range []string{"first", "second"} {
		appConfigPath := filepath.Join(root, dependent, config.DefaultTerragruntConfigPath)
		require.NoError(t, vfs.WriteFile(fsys, appConfigPath, []byte(singleOutputDependentConfig), 0o644))

		v := venv.MemVenv().
			WithFS(fsys).
			WithExec(vexec.NewMemExec(tofuOutputStub(nil))).
			WithEnv(map[string]string{"MOCK_VPC_ID": dependent})
		_, pctx := newDependentParsingContext(t, appConfigPath, "plan")
		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

		cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
		require.NoError(t, err)
		assert.Equal(t, dependent, cfg.Inputs["vpc_id"])
	}
}

// TestDependencyDeclaredOutputsEvaluateWithoutTheRoleOfTheReader expects the role of the dependent to stay unassumed.
func TestDependencyDeclaredOutputsEvaluateWithoutTheRoleOfTheReader(t *testing.T) {
	t.Parallel()

	const vpcConfig = `
outputs {
  output "vpc_id" {
    mock_value = get_aws_account_id()
  }
}
`

	root := venvtest.Root("/fixture")
	fsys := vfs.NewMemMapFS()
	vpcDir := filepath.Join(root, "vpc")
	appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
	appConfig := strings.Replace(dependentConfigWithoutInputs, "%s", "skip_outputs = true", 1) + `
inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
`
	files := map[string]string{
		filepath.Join(vpcDir, config.DefaultTerragruntConfigPath): vpcConfig,
		appConfigPath: appConfig,
	}

	for path, content := range files {
		require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
	}

	var (
		mu      sync.Mutex
		actions []string
	)

	sts := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}

		form, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}

		mu.Lock()
		defer mu.Unlock()

		actions = append(actions, form.Get("Action"))

		return vhttp.Respond(http.StatusOK, []byte(stsCallerIdentity), nil), nil
	})

	l := createLogger()
	v := venv.MemVenv().WithFS(fsys).WithHTTP(sts).WithEnv(map[string]string{
		"AWS_REGION":            "us-east-1",
		"AWS_ACCESS_KEY_ID":     "key-id",
		"AWS_SECRET_ACCESS_KEY": "secret",
	})
	ctx, pctx := newDependentParsingContext(t, appConfigPath, "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	pctx.IAMRoleOptions = iam.RoleOptions{
		RoleARN:          "arn:aws:iam::111111111111:role/reader",
		WebIdentityToken: "reader-token",
	}

	cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "222222222222", cfg.Inputs["vpc_id"])

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, []string{"GetCallerIdentity"}, actions)
}

// TestDependencyDeclaredOutputsConcurrentReadsWithRacing expects each concurrent reader to get its own copy.
func TestDependencyDeclaredOutputsConcurrentReadsWithRacing(t *testing.T) {
	t.Parallel()

	const readers = 8

	root := venvtest.Root("/fixture")
	fsys := vfs.NewMemMapFS()
	vpcDir := filepath.Join(root, "vpc")
	appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
	files := map[string]string{
		filepath.Join(vpcDir, config.DefaultTerragruntConfigPath): outputsBlockConfig,
		filepath.Join(vpcDir, "main.tf"):                          vpcModule,
		appConfigPath:                                             singleOutputDependentConfig,
	}

	for path, content := range files {
		require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
	}

	l := createLogger()
	v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(nil)))
	ctx, pctx := newDependentParsingContext(t, appConfigPath, "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
	require.NoError(t, err)
	require.Len(t, cfg.TerragruntDependencies, 1)

	dep := &cfg.TerragruntDependencies[0]
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
		assert.Equal(t, []string{"vpc_id", "subnet_ids", "undocumented"}, results[i].Names())

		if i > 0 {
			assert.NotSame(t, results[0], results[i])
		}
	}
}

// TestOutputsBlockRendersEmptyAllowedCommands expects an empty allowed_commands list to render as an empty list.
func TestOutputsBlockRendersEmptyAllowedCommands(t *testing.T) {
	t.Parallel()

	cfg := parseOutputsConfig(t, `
outputs {
  mock_strategy {
    allowed_commands = []
  }

  output "id" {
    mock_value = "mock-id"
  }
}
`, true)

	require.NotNil(t, cfg.Outputs.MockStrategy)
	require.NotNil(t, cfg.Outputs.MockStrategy.AllowedCommands)
	assert.Empty(t, *cfg.Outputs.MockStrategy.AllowedCommands)

	var rendered bytes.Buffer

	_, err := cfg.WriteTo(&rendered)
	require.NoError(t, err)
	assert.Contains(t, rendered.String(), "allowed_commands = []")

	roundTrip := parseOutputsConfig(t, rendered.String(), true)
	assert.Equal(t, cfg.Outputs, roundTrip.Outputs)

	ctyVal, err := config.TerragruntConfigAsCty(cfg)
	require.NoError(t, err)

	strategy := ctyVal.GetAttr(config.MetadataOutputs).GetAttr(config.MetadataMockStrategy)
	assert.Equal(t, cty.ListValEmpty(cty.String), strategy.GetAttr("allowed_commands"))
}

// TestOutputsBlockRendersSensitiveMockValue expects a sensitive() mock value rendered plain and marked as a value.
func TestOutputsBlockRendersSensitiveMockValue(t *testing.T) {
	t.Parallel()

	cfg := parseOutputsConfig(t, `
outputs {
  output "token" {
    mock_value = sensitive("mock-token")
  }
}
`, true)

	require.True(t, cfg.Outputs.Outputs[0].MockValue.IsMarked())

	var rendered bytes.Buffer

	_, err := cfg.WriteTo(&rendered)
	require.NoError(t, err)
	assert.Contains(t, rendered.String(), `mock_value = "mock-token"`)

	ctyVal, err := config.TerragruntConfigAsCty(cfg)
	require.NoError(t, err)

	mockValue := ctyVal.GetAttr(config.MetadataOutputs).
		GetAttr(config.MetadataOutput).
		GetAttr("token").
		GetAttr("mock_value")
	assert.True(t, mockValue.IsMarked(), "the value read_terragrunt_config exposes keeps the mark")
}

// parseOutputsConfig parses content as a unit config, with the outputs-block experiment on when enabled.
func parseOutputsConfig(t *testing.T, content string, enabled bool) *config.TerragruntConfig {
	t.Helper()

	l := createLogger()
	v := venv.MemVenv()
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

// tofuOutputStub answers `tofu output -json` from state by unit directory and succeeds silently for other commands.
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

const stsCallerIdentity = `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:sts::222222222222:assumed-role/unit/session</Arn>
    <UserId>AROATEST:session</UserId>
    <Account>222222222222</Account>
  </GetCallerIdentityResult>
</GetCallerIdentityResponse>`
