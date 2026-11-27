package config_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const typedOutputsConfig = `
outputs {
  mock_strategy {
    allowed_commands = ["validate", "plan"]
  }

  output "vpc_id" {
    type       = string
    mock_value = "vpc-mock"
  }

  output "subnet_ids" {
    type       = list(string)
    mock_value = ["subnet-mock-1", "subnet-mock-2"]
  }

  output "port" {
    type       = string
    mock_value = 8080

    mock_strategy {
      allowed_commands = ["validate"]
      merge_with_state = "shallow"
    }
  }
}
`

const secretOutputsConfig = `
outputs {
  mock_strategy {
    allowed_commands = [%s]
    %s
  }

  output "vpc_id" {
    mock_value = "vpc-mock"
    %s
  }

  output "secret" {
    mock_value = "secret-mock"
    %s
  }
}
`

const secretInputs = `
inputs = {
  vpc_id = try(dependency.vpc.outputs.vpc_id, "absent")
  secret = try(dependency.vpc.outputs.secret, "absent")
}
`

const portInputs = `
inputs = {
  port = dependency.vpc.outputs.port
}
`

const vpcStateWithOnlyVPCID = `{
  "vpc_id": {"sensitive": false, "type": "string", "value": "vpc-real"}
}`

func TestParseOutputTypeConvertsMockValue(t *testing.T) {
	t.Parallel()

	cfg := parseOutputsConfig(t, typedOutputsConfig, true)

	subnets := cfg.Outputs.Declaration("subnet_ids")
	require.NotNil(t, subnets)
	assert.Equal(t, cty.List(cty.String), subnets.Type)
	require.NotNil(t, subnets.MockValue)
	assert.True(t, subnets.MockValue.RawEquals(cty.ListVal([]cty.Value{
		cty.StringVal("subnet-mock-1"),
		cty.StringVal("subnet-mock-2"),
	})), "the tuple converts to a list: %#v", *subnets.MockValue)

	port := cfg.Outputs.Declaration("port")
	require.NotNil(t, port)
	require.NotNil(t, port.MockValue)
	assert.True(t, port.MockValue.RawEquals(cty.StringVal("8080")), "the number converts to a string")

	untyped := parseOutputsConfig(t, outputsBlockConfig, true).Outputs.Declaration("vpc_id")
	require.NotNil(t, untyped)
	assert.Equal(t, cty.NilType, untyped.Type)
	assert.Nil(t, cfg.Outputs.Declaration("nat_id"))
}

func TestParseOutputMockStrategy(t *testing.T) {
	t.Parallel()

	cfg := parseOutputsConfig(t, typedOutputsConfig, true)

	port := cfg.Outputs.Declaration("port")
	require.NotNil(t, port)
	require.NotNil(t, port.MockStrategy)
	require.NotNil(t, port.MockStrategy.AllowedCommands)
	assert.Equal(t, []string{"validate"}, *port.MockStrategy.AllowedCommands)
	require.NotNil(t, port.MockStrategy.MergeWithState)
	assert.Equal(t, config.ShallowMerge, *port.MockStrategy.MergeWithState)

	vpcID := cfg.Outputs.Declaration("vpc_id")
	require.NotNil(t, vpcID)
	assert.Nil(t, vpcID.MockStrategy)
}

func TestParseOutputRejectsMalformedTypeAndStrategy(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		check  func(t *testing.T, err error)
		name   string
		config string
	}{
		{
			name: "mock value that does not convert to the type",
			config: `
outputs {
  output "subnet_ids" {
    type       = list(string)
    mock_value = { a = "b" }
  }
}
`,
			check: func(t *testing.T, err error) {
				t.Helper()

				requireDiagnostic(t, err, "Invalid mock value")
			},
		},
		{
			name: "type that is not a type constraint",
			config: `
outputs {
  output "vpc_id" {
    type = text
  }
}
`,
			check: func(t *testing.T, err error) {
				t.Helper()

				requireDiagnostic(t, err, "Invalid type specification")
			},
		},
		{
			name: "merge strategy of an output that is not a mock merge strategy",
			config: `
outputs {
  output "vpc_id" {
    mock_strategy {
      merge_with_state = "deep"
    }
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
			name: "unknown attribute in the mock_strategy of an output",
			config: `
outputs {
  output "vpc_id" {
    mock_strategy {
      commands = ["plan"]
    }
  }
}
`,
			check: func(t *testing.T, err error) {
				t.Helper()

				requireDiagnostic(t, err, "Unsupported argument")
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

			_, err := resolveRunConfigString(
				t,
				ctx,
				l,
				v,
				new(config.Store),
				pctx,
				config.DefaultTerragruntConfigPath,
				tc.config,
			)
			require.Error(t, err)
			tc.check(t, err)
		})
	}
}

func TestOutputTypeAndStrategyRender(t *testing.T) {
	t.Parallel()

	cfg := parseOutputsConfig(t, typedOutputsConfig, true)

	var rendered bytes.Buffer

	n, err := cfg.WriteTo(&rendered)
	require.NoError(t, err)
	assert.Positive(t, n)

	roundTrip := parseOutputsConfig(t, rendered.String(), true)
	assert.Equal(t, cfg.Outputs, roundTrip.Outputs)

	ctyVal, err := config.TerragruntConfigAsCty(cfg)
	require.NoError(t, err)

	asMap, err := ctyhelper.ParseCtyValueToMap(ctyVal)
	require.NoError(t, err)

	outputs, ok := asMap[config.MetadataOutputs].(map[string]any)
	require.True(t, ok)

	declarations, ok := outputs[config.MetadataOutput].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{
		"type":       "string",
		"mock_value": "8080",
		config.MetadataMockStrategy: map[string]any{
			"allowed_commands": []any{"validate"},
			"merge_with_state": "shallow",
		},
	}, declarations["port"])
	assert.Equal(t, map[string]any{
		"type":       "list(string)",
		"mock_value": []any{"subnet-mock-1", "subnet-mock-2"},
	}, declarations["subnet_ids"])
}

func TestOutputsConfigMergeKeepsTypeAndStrategy(t *testing.T) {
	t.Parallel()

	target := &config.OutputsConfig{Outputs: []*config.OutputDeclaration{{Name: "vpc_id"}}}
	source := &config.OutputsConfig{Outputs: []*config.OutputDeclaration{{
		Name:         "vpc_id",
		Type:         cty.String,
		MockStrategy: &config.OutputsMockStrategy{AllowedCommands: new([]string{"plan"})},
	}}}

	target.Merge(source)

	merged := target.Declaration("vpc_id")
	require.NotNil(t, merged)
	assert.Equal(t, cty.String, merged.Type)
	require.NotNil(t, merged.MockStrategy)
	assert.Equal(t, []string{"plan"}, *merged.MockStrategy.AllowedCommands)
	assert.NotSame(t, source.Outputs[0].MockStrategy, merged.MockStrategy, "the merged strategy is a copy")
}

func TestDependencyReadsOutputMockStrategies(t *testing.T) {
	t.Parallel()

	strict := `mock_strategy {
      allowed_commands = ["validate"]
    }`
	broad := `mock_strategy {
      allowed_commands = ["validate", "plan"]
    }`
	shallow := `mock_strategy {
      merge_with_state = "shallow"
    }`
	noMerge := `mock_strategy {
      merge_with_state = "no_merge"
    }`

	testCases := []struct {
		check func(t *testing.T, cfg *config.TerragruntConfig, err error)
		name  string
		run   dependentRun
	}{
		{
			name: "an output with stricter allowed_commands gives no mock to the other commands",
			run: dependentRun{
				vpcConfig: secretOutputs(`"validate", "plan"`, "", "", strict),
				command:   "plan",
			},
			check: expectSecretInputs("vpc-mock", "absent"),
		},
		{
			name: "an output with stricter allowed_commands gives its mock to its commands",
			run: dependentRun{
				vpcConfig: secretOutputs(`"validate", "plan"`, "", "", strict),
				command:   "validate",
			},
			check: expectSecretInputs("vpc-mock", "secret-mock"),
		},
		{
			name: "an output with broader allowed_commands gives its mock alone",
			run: dependentRun{
				vpcConfig: secretOutputs(`"validate"`, "", broad, ""),
				command:   "plan",
			},
			check: expectSecretInputs("vpc-mock", "absent"),
		},
		{
			name: "no mock stands in when no output allows the command",
			run: dependentRun{
				vpcConfig: secretOutputs(`"validate"`, "", strict, ""),
				command:   "plan",
			},
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var noOutputsErr config.TerragruntOutputTargetNoOutputs

				require.ErrorAs(t, err, &noOutputsErr)
			},
		},
		{
			name: "the dependent's allowed commands win over the output's",
			run: dependentRun{
				vpcConfig:      secretOutputs(`"validate", "plan"`, "", "", strict),
				dependencyBody: `mock_outputs_allowed_terraform_commands = ["plan"]`,
				command:        "plan",
			},
			check: expectSecretInputs("vpc-mock", "secret-mock"),
		},
		{
			name: "the dependent's mock for an output follows the strategy of that output",
			run: dependentRun{
				vpcConfig:      secretOutputs(`"validate", "plan"`, "", "", strict),
				dependencyBody: `mock_outputs = { secret = "app-secret" }`,
				command:        "plan",
			},
			check: expectSecretInputs("vpc-mock", "absent"),
		},
		{
			name: "the dependent's mock for an output stands in for a command the output allows",
			run: dependentRun{
				vpcConfig:      secretOutputs(`"validate", "plan"`, "", "", strict),
				dependencyBody: `mock_outputs = { secret = "app-secret" }`,
				command:        "validate",
			},
			check: expectSecretInputs("vpc-mock", "app-secret"),
		},
		{
			name: "an output merges with state by its own strategy",
			run: dependentRun{
				vpcConfig: secretOutputs(`"validate", "plan"`, "", "", shallow),
				command:   "plan",
				vpcState:  vpcStateWithOnlyVPCID,
			},
			check: expectSecretInputs("vpc-real", "secret-mock"),
		},
		{
			name: "an output that opts out of the merge is missing from state",
			run: dependentRun{
				vpcConfig: secretOutputs(`"validate", "plan"`, `merge_with_state = "shallow"`, "", noMerge),
				command:   "plan",
				vpcState:  vpcStateWithOnlyVPCID,
			},
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var missingErr config.DeclaredOutputMissingError

				require.ErrorAs(t, err, &missingErr)
				assert.Equal(t, "secret", missingErr.Output)
			},
		},
		{
			name: "the dependent's merge strategy wins over the output's",
			run: dependentRun{
				vpcConfig:      secretOutputs(`"validate", "plan"`, "", "", noMerge),
				dependencyBody: `mock_outputs_merge_strategy_with_state = "shallow"`,
				command:        "plan",
				vpcState:       vpcStateWithOnlyVPCID,
			},
			check: expectSecretInputs("vpc-real", "secret-mock"),
		},
		{
			name: "a merge of an output does not run for a command the output excludes",
			run: dependentRun{
				vpcConfig: secretOutputs(`"validate", "plan"`, `merge_with_state = "shallow"`, "", strict),
				command:   "plan",
				vpcState:  vpcStateWithOnlyVPCID,
			},
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var missingErr config.DeclaredOutputMissingError

				require.ErrorAs(t, err, &missingErr)
				assert.Equal(t, "secret", missingErr.Output)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.run.inputs = secretInputs

			cfg, err := resolveDependent(t, &tc.run)
			tc.check(t, cfg, err)
		})
	}
}

func TestDependencyReadsTypedOutputs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		check func(t *testing.T, cfg *config.TerragruntConfig, err error)
		name  string
		run   dependentRun
	}{
		{
			name: "a typed mock value reaches the dependent converted",
			run:  dependentRun{vpcConfig: typedOutputsConfig, command: "validate"},
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "8080", cfg.Inputs["port"])
			},
		},
		{
			name: "a state output converts to its declared type",
			run: dependentRun{
				vpcConfig: typedOutputsConfig,
				command:   "plan",
				vpcState: `{
  "vpc_id": {"sensitive": false, "type": "string", "value": "vpc-real"},
  "subnet_ids": {"sensitive": false, "type": ["tuple", ["string"]], "value": ["subnet-real"]},
  "port": {"sensitive": false, "type": "number", "value": 443}
}`,
			},
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "443", cfg.Inputs["port"])
			},
		},
		{
			name: "a state output that does not convert to its declared type is an error",
			run: dependentRun{
				vpcConfig: typedOutputsConfig,
				command:   "plan",
				vpcState: `{
  "vpc_id": {"sensitive": false, "type": "string", "value": "vpc-real"},
  "subnet_ids": {"sensitive": false, "type": ["object", {"a": "string"}], "value": {"a": "subnet-real"}},
  "port": {"sensitive": false, "type": "number", "value": 443}
}`,
			},
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var typeErr config.DeclaredOutputTypeError

				require.ErrorAs(t, err, &typeErr)
				assert.Equal(t, "subnet_ids", typeErr.Output)
				assert.Equal(t, "list(string)", typeErr.Type)
				assert.True(t, strings.HasSuffix(typeErr.TargetConfig, filepath.Join("vpc", config.DefaultTerragruntConfigPath)))

				var blockErr config.DependencyBlockError

				require.ErrorAs(t, err, &blockErr)
				assert.Equal(t, "vpc", blockErr.Name)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.run.inputs = portInputs

			cfg, err := resolveDependent(t, &tc.run)
			tc.check(t, cfg, err)
		})
	}
}

// dependentRun is a unit app that reads the unit vpc through a dependency block.
type dependentRun struct {
	vpcConfig      string
	dependencyBody string
	inputs         string
	command        string
	vpcState       string
}

// resolveDependent resolves the run config of the app unit of run, with the state of vpc answered from run.vpcState.
func resolveDependent(t *testing.T, run *dependentRun) (*config.TerragruntConfig, error) {
	t.Helper()

	root := venvtest.Root("/fixture")
	fsys := vfs.NewMemMapFS()
	vpcDir := filepath.Join(root, "vpc")
	appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
	appConfig := strings.Replace(dependentConfigWithoutInputs, "%s", run.dependencyBody, 1) + run.inputs

	files := map[string]string{
		filepath.Join(vpcDir, config.DefaultTerragruntConfigPath): run.vpcConfig,
		filepath.Join(vpcDir, "main.tf"):                          vpcModule,
		appConfigPath:                                             appConfig,
	}

	for path, content := range files {
		require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
	}

	state := map[string]string{}
	if run.vpcState != "" {
		state[vpcDir] = run.vpcState
	}

	l := createLogger()
	v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(state)))
	ctx, pctx := newDependentParsingContext(t, appConfigPath, run.command)
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

	return new(config.Store).ResolveRunConfig(ctx, l, v, pctx, appConfigPath)
}

// secretOutputs returns an outputs block of vpc_id and secret with the given commands, merge, and output strategies.
func secretOutputs(commands, merge, vpcStrategy, secretStrategy string) string {
	block := strings.Replace(secretOutputsConfig, "%s", commands, 1)
	block = strings.Replace(block, "%s", merge, 1)
	block = strings.Replace(block, "%s", vpcStrategy, 1)

	return strings.Replace(block, "%s", secretStrategy, 1)
}

// expectSecretInputs returns a check that the dependent resolved vpc_id and secret to the given values.
func expectSecretInputs(vpcID, secret string) func(t *testing.T, cfg *config.TerragruntConfig, err error) {
	return func(t *testing.T, cfg *config.TerragruntConfig, err error) {
		t.Helper()

		require.NoError(t, err)
		assert.Equal(t, vpcID, cfg.Inputs["vpc_id"])
		assert.Equal(t, secret, cfg.Inputs["secret"])
	}
}
