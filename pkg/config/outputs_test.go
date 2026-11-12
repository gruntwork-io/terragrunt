package config_test

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

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
	"github.com/gruntwork-io/terragrunt/pkg/log"
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
			name: "a reader that ignores diagnostics still gets the error of a broken unit config",
			vpcConfig: `
outputs {
  output "vpc_id" {
`,
			command:           "plan",
			ignoreDiagnostics: true,
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var readErr config.DeclaredOutputsReadError

				require.ErrorAs(t, err, &readErr)
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

func TestDependencyReadGivesOutputsBlockTip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		vpcConfig     string
		experiments   []string
		expectedTipAt []string
	}{
		{
			name:          "a unit without an outputs block gets the tip",
			vpcConfig:     ``,
			experiments:   []string{experiment.OutputsBlock},
			expectedTipAt: []string{filepath.Join("vpc", config.DefaultTerragruntConfigPath)},
		},
		{
			name:        "a unit with an outputs block gets no tip",
			vpcConfig:   outputsBlockConfig,
			experiments: []string{experiment.OutputsBlock},
		},
		{
			name:      "without the experiment there is no tip",
			vpcConfig: ``,
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

// TestDependencyReadsDeclaredOutputsThroughInclude expects an outputs block merged from an include to reach dependents.
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
	v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(nil)))
	ctx, pctx := newDependentParsingContext(t, appConfigPath, "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "vpc-mock", cfg.Inputs["vpc_id"])
	assert.Equal(t, []any{"subnet-mock-1", "subnet-mock-2"}, cfg.Inputs["subnet_ids"])
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

	tipped := false

	pctx.OutputsBlockTip = func(log.Logger, string) {
		tipped = true
	}

	cfg, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "stack-mock", cfg.Inputs["vpc_id"])
	assert.False(t, tipped, "a stack target gets no outputs-block tip")
}

// TestDependencyDeclaredOutputsReadsEachUnitOnce expects one evaluation per unit config until the file changes.
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
	v := venv.MemVenv().WithFS(frozenModTimeFS{FS: fsys, modTime: time.Now().Add(-time.Hour)})
	ctx, pctx := newDependentParsingContext(t, filepath.Join(root, "app", config.DefaultTerragruntConfigPath), "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	dep := config.Dependency{Name: "vpc", ConfigPath: cty.StringVal("../vpc")}

	first, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)

	evaluated := fsys.count(tfvarsPath)

	second, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)

	require.NotNil(t, first)
	assert.Equal(t, "from-tfvars", first.Outputs[0].MockValue.AsString())
	assert.Equal(t, first.Names(), second.Names())
	assert.NotSame(t, first, second, "each read gets its own copy")
	assert.Positive(t, evaluated)
	assert.Equal(t, evaluated, fsys.count(tfvarsPath), "the second read came from the cache")

	require.NoError(t, vfs.WriteFile(fsys, vpcConfigPath, []byte(vpcConfig+"\n"), 0o644))

	third, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)
	assert.Equal(t, first.Names(), third.Names())
	assert.Greater(t, fsys.count(tfvarsPath), evaluated, "a changed unit config is read again")

	require.NoError(t, vfs.WriteFile(fsys, tfvarsPath, []byte(`mock = "from-tfvars-2"`), 0o644))

	fourth, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)
	require.NotNil(t, fourth)
	assert.Equal(
		t,
		"from-tfvars-2",
		fourth.Outputs[0].MockValue.AsString(),
		"a changed file the config reads is read again",
	)
}

// TestDependencyDeclaredOutputsReplaysFilesRead expects a cached read to record the files the parse took.
func TestDependencyDeclaredOutputsReplaysFilesRead(t *testing.T) {
	t.Parallel()

	const vpcConfig = `
locals {
  vars = jsondecode(read_tfvars_file("vpc.tfvars"))
}

outputs {
  output "vpc_id" {
    mock_value = local.vars.mock
  }
}
`

	root := venvtest.Root("/fixture")
	fsys := vfs.NewMemMapFS()
	vpcDir := filepath.Join(root, "vpc")
	vpcConfigPath := filepath.Join(vpcDir, config.DefaultTerragruntConfigPath)
	tfvarsPath := filepath.Join(vpcDir, "vpc.tfvars")

	require.NoError(t, vfs.WriteFile(fsys, vpcConfigPath, []byte(vpcConfig), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, tfvarsPath, []byte(`mock = "from-tfvars"`), 0o644))

	l := createLogger()
	v := venv.MemVenv().WithFS(fsys)
	ctx, pctx := newDependentParsingContext(t, filepath.Join(root, "app", config.DefaultTerragruntConfigPath), "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	dep := config.Dependency{Name: "vpc", ConfigPath: cty.StringVal("../vpc")}

	for _, reader := range []string{"first", "second"} {
		readerPctx := pctx.Clone()
		readerPctx.FilesRead = config.NewFilesRead()

		_, err := dep.DeclaredOutputs(ctx, l, v, readerPctx)
		require.NoError(t, err)

		assert.Equal(t, []string{vpcConfigPath, tfvarsPath}, readerPctx.FilesRead.Paths(), reader)
	}
}

// TestDependencyDeclaredOutputsRefreshesIncludedDeclarations expects a refresh to read the included files again.
func TestDependencyDeclaredOutputsRefreshesIncludedDeclarations(t *testing.T) {
	t.Parallel()

	const rootConfig = `
locals {
  vars = jsondecode(read_tfvars_file("${get_terragrunt_dir()}/vpc.tfvars"))
}

outputs {
  output "vpc_id" {
    mock_value = local.vars.mock
  }
}
`

	const vpcConfig = `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`

	root := venvtest.Root("/fixture")
	fsys := frozenModTimeFS{FS: vfs.NewMemMapFS(), modTime: time.Now()}
	vpcDir := filepath.Join(root, "vpc")
	vpcConfigPath := filepath.Join(vpcDir, config.DefaultTerragruntConfigPath)
	tfvarsPath := filepath.Join(vpcDir, "vpc.tfvars")

	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "root.hcl"), []byte(rootConfig), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, vpcConfigPath, []byte(vpcConfig), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, tfvarsPath, []byte(`mock = "vpc-mock-a"`), 0o644))

	l := createLogger()
	v := venv.MemVenv().WithFS(fsys)
	ctx, pctx := newDependentParsingContext(t, filepath.Join(root, "app", config.DefaultTerragruntConfigPath), "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	pctx.UsePartialParseConfigCache = true

	dep := config.Dependency{Name: "vpc", ConfigPath: cty.StringVal("../vpc")}

	first, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "vpc-mock-a", first.Outputs[0].MockValue.AsString())

	require.NoError(t, vfs.WriteFile(fsys, tfvarsPath, []byte(`mock = "vpc-mock-b"`), 0o644))

	second, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, "vpc-mock-b", second.Outputs[0].MockValue.AsString())
}

// TestDependencyDeclaredOutputsReadsARewrittenAutoinclude expects an edited autoinclude file to be read again.
func TestDependencyDeclaredOutputsReadsARewrittenAutoinclude(t *testing.T) {
	t.Parallel()

	const autoInclude = `
outputs {
  output "vpc_id" {
    mock_value = "vpc-mock-%s"
  }
}
`

	root := venvtest.Root("/fixture")
	fsys := frozenModTimeFS{FS: vfs.NewMemMapFS(), modTime: time.Now()}
	vpcDir := filepath.Join(root, "vpc")
	autoIncludePath := filepath.Join(vpcDir, "terragrunt.autoinclude.hcl")

	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(vpcDir, config.DefaultTerragruntConfigPath), []byte("\n"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, autoIncludePath, fmt.Appendf(nil, autoInclude, "a"), 0o644))

	l := createLogger()
	v := venv.MemVenv().WithFS(fsys)
	ctx, pctx := newDependentParsingContext(t, filepath.Join(root, "app", config.DefaultTerragruntConfigPath), "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	pctx.FilesRead = config.NewFilesRead()

	dep := config.Dependency{Name: "vpc", ConfigPath: cty.StringVal("../vpc")}

	first, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "vpc-mock-a", first.Outputs[0].MockValue.AsString())
	assert.Contains(t, pctx.FilesRead.Paths(), autoIncludePath)

	require.NoError(t, vfs.WriteFile(fsys, autoIncludePath, fmt.Appendf(nil, autoInclude, "b"), 0o644))

	second, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, "vpc-mock-b", second.Outputs[0].MockValue.AsString())
}

// TestDependencyDeclaredOutputsMatchAfterADependentParse expects a dependent parse to leave what a read gives.
func TestDependencyDeclaredOutputsMatchAfterADependentParse(t *testing.T) {
	t.Parallel()

	const rootInclude = `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`

	const rootLocals = `
locals {
  region = "us-east-1"
}
`

	testCases := []struct {
		files map[string]string
		name  string
		fails bool
	}{
		{
			name:  "a unit with no outputs block",
			files: map[string]string{"vpc/terragrunt.hcl": rootLocals},
		},
		{
			name:  "an including unit with no outputs block",
			files: map[string]string{"root.hcl": rootLocals, "vpc/terragrunt.hcl": rootInclude},
		},
		{
			name:  "constant outputs",
			files: map[string]string{"root.hcl": rootLocals, "vpc/terragrunt.hcl": rootInclude + outputsBlockConfig},
		},
		{
			name: "outputs merged with the outputs of an include",
			files: map[string]string{
				"root.hcl": `
outputs {
  output "region" {
    mock_value = "us-east-1"
  }
}
`,
				"vpc/terragrunt.hcl": rootInclude + outputsBlockConfig,
			},
		},
		{
			name:  "outputs of an include alone",
			files: map[string]string{"root.hcl": outputsBlockConfig, "vpc/terragrunt.hcl": rootInclude},
		},
		{
			name: "a mock value read from a local",
			files: map[string]string{
				"vpc/terragrunt.hcl": `
locals {
  id = "vpc-local"
}

outputs {
  output "vpc_id" {
    mock_value = local.id
  }
}
`,
			},
		},
		{
			name: "a sensitive mock value",
			files: map[string]string{
				"vpc/terragrunt.hcl": `
outputs {
  output "vpc_id" {
    mock_value = sensitive("vpc-secret")
  }
}
`,
			},
		},
		{
			name: "outputs of the autoinclude file",
			files: map[string]string{
				"vpc/terragrunt.hcl":             "\n",
				"vpc/terragrunt.autoinclude.hcl": outputsBlockConfig,
			},
		},
		{
			name: "an output declared twice",
			files: map[string]string{
				"vpc/terragrunt.hcl": `
outputs {
  output "vpc_id" {}

  output "vpc_id" {}
}
`,
			},
			fails: true,
		},
		{
			name: "an unclosed outputs block",
			files: map[string]string{
				"vpc/terragrunt.hcl": `
outputs {
  output "vpc_id" {
`,
			},
			fails: true,
		},
		{
			name: "a failing local beside constant outputs",
			files: map[string]string{
				"vpc/terragrunt.hcl": `
locals {
  absent = file("absent.txt")
}
` + outputsBlockConfig,
			},
			fails: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			fsys := vfs.NewMemMapFS()
			appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
			appConfig := strings.Replace(dependentConfigWithoutInputs, "%s", `mock_outputs = { vpc_id = "app-mock" }`, 1)

			require.NoError(t, vfs.WriteFile(fsys, appConfigPath, []byte(appConfig), 0o644))
			require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "vpc", "main.tf"), []byte(vpcModule), 0o644))

			for path, content := range tc.files {
				require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, filepath.FromSlash(path)), []byte(content), 0o644))
			}

			l := createLogger()
			v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(nil)))
			dep := config.Dependency{Name: "vpc", ConfigPath: cty.StringVal("../vpc")}

			readCtx, readPctx := newDependentParsingContext(t, appConfigPath, "plan")
			require.NoError(t, readPctx.Experiments.EnableExperiment(experiment.OutputsBlock))

			read, readErr := dep.DeclaredOutputs(readCtx, l, v, readPctx)

			parsedCtx, parsedPctx := newDependentParsingContext(t, appConfigPath, "plan")
			require.NoError(t, parsedPctx.Experiments.EnableExperiment(experiment.OutputsBlock))

			_, parseErr := config.ParseConfigFile(parsedCtx, l, v, parsedPctx, appConfigPath, nil)
			afterParse, afterParseErr := dep.DeclaredOutputs(parsedCtx, l, v, parsedPctx)

			if tc.fails {
				var blockErr config.DependencyBlockError

				require.Error(t, parseErr)
				require.ErrorAs(t, readErr, &blockErr)
				require.ErrorAs(t, afterParseErr, &blockErr)

				return
			}

			require.NoError(t, readErr)
			require.NoError(t, parseErr)
			require.NoError(t, afterParseErr)
			assert.Equal(t, read, afterParse)
		})
	}
}

// TestDependencyConstantDeclaredOutputsTakeNoEvaluation expects a dependent parse to settle them off the syntax tree.
func TestDependencyConstantDeclaredOutputsTakeNoEvaluation(t *testing.T) {
	t.Parallel()

	const vpcConfig = `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

locals {
  vars = jsondecode(read_tfvars_file("vpc.tfvars"))
}
`

	testCases := []struct {
		name      string
		outputs   string
		evaluates bool
	}{
		{name: "no outputs block"},
		{name: "constant outputs", outputs: outputsBlockConfig},
		{
			name: "a mock value read from a local",
			outputs: `
outputs {
  output "vpc_id" {
    mock_value = local.vars.name
  }
}
`,
			evaluates: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			fsys := vfs.NewMemMapFS()
			vpcDir := filepath.Join(root, "vpc")
			tfvarsPath := filepath.Join(vpcDir, "vpc.tfvars")
			appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
			appConfig := strings.Replace(dependentConfigWithoutInputs, "%s", `mock_outputs = { vpc_id = "app-mock" }`, 1)
			files := map[string]string{
				filepath.Join(root, "root.hcl"):                           "\n",
				filepath.Join(vpcDir, config.DefaultTerragruntConfigPath): vpcConfig + tc.outputs,
				filepath.Join(vpcDir, "main.tf"):                          vpcModule,
				tfvarsPath:                                                `name = "vpc"`,
				appConfigPath:                                             appConfig,
			}

			for path, content := range files {
				require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
			}

			l := createLogger()
			v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(nil)))
			ctx, pctx := newDependentParsingContext(t, appConfigPath, "plan")
			require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

			_, err := config.ParseConfigFile(ctx, l, v, pctx, appConfigPath, nil)
			require.NoError(t, err)

			readerPctx := pctx.Clone()
			readerPctx.FilesRead = config.NewFilesRead()

			dep := config.Dependency{Name: "vpc", ConfigPath: cty.StringVal("../vpc")}

			_, err = dep.DeclaredOutputs(ctx, l, v, readerPctx)
			require.NoError(t, err)

			assert.Equal(t, tc.evaluates, slices.Contains(readerPctx.FilesRead.Paths(), tfvarsPath))
		})
	}
}

// TestDependencyEvaluatedDeclaredOutputsFollowTheReaderEnvironment expects no reader to get another reader's value.
func TestDependencyEvaluatedDeclaredOutputsFollowTheReaderEnvironment(t *testing.T) {
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
	vpcConfigPath := filepath.Join(root, "vpc", config.DefaultTerragruntConfigPath)

	require.NoError(t, vfs.WriteFile(fsys, vpcConfigPath, []byte(vpcConfig), 0o644))

	l := createLogger()
	ctx, pctx := newDependentParsingContext(t, filepath.Join(root, "app", config.DefaultTerragruntConfigPath), "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	dep := config.Dependency{Name: "vpc", ConfigPath: cty.StringVal("../vpc")}

	for _, reader := range []string{"first", "second"} {
		v := venv.MemVenv().WithFS(fsys).WithEnv(map[string]string{"MOCK_VPC_ID": reader})

		declared, err := dep.DeclaredOutputs(ctx, l, v, pctx)
		require.NoError(t, err)
		require.NotNil(t, declared)
		assert.Equal(t, reader, declared.Outputs[0].MockValue.AsString())
	}
}

// TestDependencyDeclaredOutputsEvaluateUnderTheRoleOfTheirUnit expects the dependency's iam_role, not the reader's.
func TestDependencyDeclaredOutputsEvaluateUnderTheRoleOfTheirUnit(t *testing.T) {
	t.Parallel()

	const (
		readerRole = "arn:aws:iam::111111111111:role/reader"
		unitRole   = "arn:aws:iam::222222222222:role/unit"
	)

	vpcConfig := `
iam_role               = "` + unitRole + `"
iam_web_identity_token = "unit-token"

outputs {
  output "account_id" {
    mock_value = get_aws_account_id()
  }
}
`

	root := venvtest.Root("/fixture")
	fsys := vfs.NewMemMapFS()
	vpcConfigPath := filepath.Join(root, "vpc", config.DefaultTerragruntConfigPath)

	require.NoError(t, vfs.WriteFile(fsys, vpcConfigPath, []byte(vpcConfig), 0o644))

	var (
		mu      sync.Mutex
		assumed []string
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

		if form.Get("Action") != "AssumeRoleWithWebIdentity" {
			return vhttp.Respond(http.StatusOK, []byte(stsCallerIdentity), nil), nil
		}

		mu.Lock()
		defer mu.Unlock()

		assumed = append(assumed, form.Get("RoleArn"))

		return vhttp.Respond(http.StatusOK, []byte(stsAssumedRole), nil), nil
	})

	l := createLogger()
	v := venv.MemVenv().WithFS(fsys).WithHTTP(sts).WithEnv(map[string]string{"AWS_REGION": "us-east-1"})
	ctx, pctx := newDependentParsingContext(t, filepath.Join(root, "app", config.DefaultTerragruntConfigPath), "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	pctx.IAMRoleOptions = iam.RoleOptions{RoleARN: readerRole, WebIdentityToken: "reader-token"}

	dep := config.Dependency{Name: "vpc", ConfigPath: cty.StringVal("../vpc")}

	declared, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)
	require.NotNil(t, declared)
	assert.Equal(t, "222222222222", declared.Outputs[0].MockValue.AsString())

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, []string{unitRole}, assumed)
}

// TestDependencyOnStackDirectoryWithUnitFileReadsNoDeclaredOutputs expects a stack target to keep its unit file out.
func TestDependencyOnStackDirectoryWithUnitFileReadsNoDeclaredOutputs(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	fsys := vfs.NewMemMapFS()
	vpcDir := filepath.Join(root, "vpc")

	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(vpcDir, config.DefaultStackFile), []byte(""), 0o644))
	require.NoError(t, vfs.WriteFile(
		fsys,
		filepath.Join(vpcDir, config.DefaultTerragruntConfigPath),
		[]byte(outputsBlockConfig),
		0o644,
	))

	l := createLogger()
	v := venv.MemVenv().WithFS(fsys)
	ctx, pctx := newDependentParsingContext(t, filepath.Join(root, "app", config.DefaultTerragruntConfigPath), "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	tipped := false
	pctx.OutputsBlockTip = func(log.Logger, string) { tipped = true }

	dep := config.Dependency{Name: "vpc", ConfigPath: cty.StringVal("../vpc")}

	declared, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)
	assert.Nil(t, declared)
	assert.False(t, tipped)
}

// TestDependencyDeclaredOutputsConcurrentReadsWithRacing expects a copy per reader and at most one parse per reader.
func TestDependencyDeclaredOutputsConcurrentReadsWithRacing(t *testing.T) {
	t.Parallel()

	const readers = 8

	root := venvtest.Root("/fixture")
	fsys := &openCountingFS{FS: vfs.NewMemMapFS(), opens: map[string]int{}}
	vpcConfigPath := filepath.Join(root, "vpc", config.DefaultTerragruntConfigPath)

	require.NoError(t, vfs.WriteFile(fsys, vpcConfigPath, []byte(outputsBlockConfig), 0o644))

	l := createLogger()
	v := venv.MemVenv().WithFS(fsys)
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

		if i > 0 {
			assert.NotSame(t, results[0], results[i])
		}
	}

	const opensPerReader = 2

	opens := fsys.count(vpcConfigPath)
	assert.GreaterOrEqual(t, opens, 1)
	assert.LessOrEqual(t, opens, readers*opensPerReader)
}

// TestDependencyDeclaredOutputsReadsARacyRewrite expects a same-size rewrite within the racy mtime window to be read.
func TestDependencyDeclaredOutputsReadsARacyRewrite(t *testing.T) {
	t.Parallel()

	const vpcConfig = `
outputs {
  output "vpc_id" {
    mock_value = "vpc-mock-%s"
  }
}
`

	root := venvtest.Root("/fixture")
	fsys := frozenModTimeFS{FS: vfs.NewMemMapFS(), modTime: time.Now()}
	vpcConfigPath := filepath.Join(root, "vpc", config.DefaultTerragruntConfigPath)

	require.NoError(t, vfs.WriteFile(fsys, vpcConfigPath, fmt.Appendf(nil, vpcConfig, "a"), 0o644))

	l := createLogger()
	v := venv.MemVenv().WithFS(fsys)
	ctx, pctx := newDependentParsingContext(t, filepath.Join(root, "app", config.DefaultTerragruntConfigPath), "plan")
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	dep := config.Dependency{Name: "vpc", ConfigPath: cty.StringVal("../vpc")}

	first, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "vpc-mock-a", first.Outputs[0].MockValue.AsString())

	require.NoError(t, vfs.WriteFile(fsys, vpcConfigPath, fmt.Appendf(nil, vpcConfig, "b"), 0o644))

	second, err := dep.DeclaredOutputs(ctx, l, v, pctx)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, "vpc-mock-b", second.Outputs[0].MockValue.AsString())
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

const stsAssumedRole = `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>assumed-key-id</AccessKeyId>
      <SecretAccessKey>assumed-secret</SecretAccessKey>
      <SessionToken>assumed-token</SessionToken>
      <Expiration>2099-01-01T00:00:00Z</Expiration>
    </Credentials>
    <AssumedRoleUser>
      <AssumedRoleId>AROATEST:session</AssumedRoleId>
      <Arn>arn:aws:sts::222222222222:assumed-role/unit/session</Arn>
    </AssumedRoleUser>
  </AssumeRoleWithWebIdentityResult>
</AssumeRoleWithWebIdentityResponse>`

const stsCallerIdentity = `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:sts::222222222222:assumed-role/unit/session</Arn>
    <UserId>AROATEST:session</UserId>
    <Account>222222222222</Account>
  </GetCallerIdentityResult>
</GetCallerIdentityResponse>`

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

type frozenModTimeFS struct {
	vfs.FS
	modTime time.Time
}

func (fsys frozenModTimeFS) Stat(name string) (fs.FileInfo, error) {
	info, err := fsys.FS.Stat(name)
	if err != nil {
		return nil, err
	}

	return frozenModTimeInfo{FileInfo: info, modTime: fsys.modTime}, nil
}

type frozenModTimeInfo struct {
	fs.FileInfo
	modTime time.Time
}

func (info frozenModTimeInfo) ModTime() time.Time {
	return info.modTime
}
