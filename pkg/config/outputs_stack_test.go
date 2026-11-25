package config_test

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const stackOfVPC = `
unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}
`

const stackDependentConfig = `
dependency "stack" {
  config_path = "../stack"
  %s
}

inputs = {
  vpc_id     = try(dependency.stack.outputs.vpc.vpc_id, "absent")
  subnet_ids = try(dependency.stack.outputs.vpc.subnet_ids, ["absent"])
}
`

func TestDependencyOnStackReadsDeclaredOutputsOfUnits(t *testing.T) {
	t.Parallel()

	const vpcStateWithBothOutputs = `{
  "vpc_id": {"sensitive": false, "type": "string", "value": "vpc-real"},
  "subnet_ids": {"sensitive": false, "type": ["tuple", ["string"]], "value": ["subnet-real"]},
  "undocumented": {"sensitive": false, "type": "string", "value": "real"}
}`

	const vpcStateWithoutSubnets = `{
  "vpc_id": {"sensitive": false, "type": "string", "value": "vpc-real"},
  "undocumented": {"sensitive": false, "type": "string", "value": "real"}
}`

	withoutMerge := strings.ReplaceAll(outputsBlockConfig, `merge_with_state = "shallow"`, "")
	mockSubnets := []any{"subnet-mock-1", "subnet-mock-2"}

	testCases := []struct {
		check          func(t *testing.T, cfg *config.TerragruntConfig, err error)
		name           string
		unitConfig     string
		dependencyBody string
		command        string
		unitState      string
		experimentOff  bool
	}{
		{
			name:       "a unit without state gives its declared mocks",
			unitConfig: outputsBlockConfig,
			command:    "plan",
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-mock", cfg.Inputs["vpc_id"])
				assert.Equal(t, mockSubnets, cfg.Inputs["subnet_ids"])
			},
		},
		{
			name:           "the dependent's mock for the unit wins key by key",
			unitConfig:     outputsBlockConfig,
			dependencyBody: `mock_outputs = { vpc = { vpc_id = "app-mock" } }`,
			command:        "plan",
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "app-mock", cfg.Inputs["vpc_id"])
				assert.Equal(t, mockSubnets, cfg.Inputs["subnet_ids"])
			},
		},
		{
			name:       "a command outside allowed_commands reads no mocks",
			unitConfig: outputsBlockConfig,
			command:    "apply",
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var noOutputsErr config.TerragruntOutputTargetNoOutputs

				require.ErrorAs(t, err, &noOutputsErr)
			},
		},
		{
			name:       "state outputs win over declared mocks",
			unitConfig: outputsBlockConfig,
			command:    "plan",
			unitState:  vpcStateWithBothOutputs,
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-real", cfg.Inputs["vpc_id"])
				assert.Equal(t, []any{"subnet-real"}, cfg.Inputs["subnet_ids"])
			},
		},
		{
			name:       "a declared output missing from the state of the unit is an error",
			unitConfig: withoutMerge,
			command:    "plan",
			unitState:  vpcStateWithoutSubnets,
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var missingErr config.DeclaredOutputMissingError

				require.ErrorAs(t, err, &missingErr)
				assert.Equal(t, "subnet_ids", missingErr.Output)

				var blockErr config.DependencyBlockError

				require.ErrorAs(t, err, &blockErr)
				assert.Equal(t, "stack", blockErr.Name)
			},
		},
		{
			name:       "the strategy of the unit fills its state from the declared mocks",
			unitConfig: outputsBlockConfig,
			command:    "plan",
			unitState:  vpcStateWithoutSubnets,
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-real", cfg.Inputs["vpc_id"])
				assert.Equal(t, mockSubnets, cfg.Inputs["subnet_ids"])
			},
		},
		{
			name:           "without the experiment the dependent reads its own mock",
			unitConfig:     ``,
			dependencyBody: `mock_outputs = { vpc = { vpc_id = "app-mock" } }`,
			command:        "plan",
			experimentOff:  true,
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "app-mock", cfg.Inputs["vpc_id"])
			},
		},
		{
			name:           "a unit without an outputs block keeps the mock of the dependent",
			unitConfig:     ``,
			dependencyBody: `mock_outputs = { vpc = { vpc_id = "app-mock" } }`,
			command:        "plan",
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "app-mock", cfg.Inputs["vpc_id"])
				assert.Equal(t, []any{"absent"}, cfg.Inputs["subnet_ids"])
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			fsys := vfs.NewMemMapFS()
			unitDir := filepath.Join(root, "stack", ".terragrunt-stack", "vpc")
			appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
			appConfig := strings.Replace(stackDependentConfig, "%s", tc.dependencyBody, 1)

			files := map[string]string{
				filepath.Join(root, "stack", config.DefaultStackFile):      stackOfVPC,
				filepath.Join(unitDir, config.DefaultTerragruntConfigPath): tc.unitConfig,
				filepath.Join(unitDir, "main.tf"):                          vpcModule,
				appConfigPath:                                              appConfig,
			}

			for path, content := range files {
				require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
			}

			state := map[string]string{}
			if tc.unitState != "" {
				state[unitDir] = tc.unitState
			}

			l := createLogger()
			v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(tofuOutputStub(state)))
			ctx, pctx := newDependentParsingContext(t, appConfigPath, tc.command)

			if !tc.experimentOff {
				require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))
			}

			cfg, err := new(config.Store).ResolveRunConfig(ctx, l, v, pctx, appConfigPath)
			tc.check(t, cfg, err)
		})
	}
}

func TestDependencyOnStackReadsDeclaredMocksOfUnitWithoutState(t *testing.T) {
	t.Parallel()

	const unitConfig = `
remote_state {
  backend = "gcs"
  config = {
    access_token = "test-token"
    bucket       = "state-bucket"
    prefix       = "environment/vpc"
  }
}
` + outputsBlockConfig

	testCases := []struct {
		check   func(t *testing.T, cfg *config.TerragruntConfig, err error)
		name    string
		command string
	}{
		{
			name:    "a command in allowed_commands reads the declared mocks",
			command: "plan",
			check: func(t *testing.T, cfg *config.TerragruntConfig, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "vpc-mock", cfg.Inputs["vpc_id"])
			},
		},
		{
			name:    "a command outside allowed_commands fails on the missing state",
			command: "apply",
			check: func(t *testing.T, _ *config.TerragruntConfig, err error) {
				t.Helper()

				var fetchErr config.StackUnitOutputFetchError

				require.ErrorAs(t, err, &fetchErr)
				assert.Equal(t, "vpc", fetchErr.UnitName)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			unitDir := filepath.Join(root, "stack", ".terragrunt-stack", "vpc")
			appConfigPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
			missing := []byte(`{"error":{"code":404,"message":"missing"}}`)
			recorder := newDependencyStateRecorder(t, http.StatusNotFound, missing)
			v := venv.MemVenv().
				WithEnv(map[string]string{}).
				WithExec(recorder.exec()).
				WithHTTP(recorder.httpClient())

			files := map[string]string{
				filepath.Join(root, "stack", config.DefaultStackFile):      stackOfVPC,
				filepath.Join(unitDir, config.DefaultTerragruntConfigPath): unitConfig,
				appConfigPath: strings.Replace(stackDependentConfig, "%s", "", 1),
			}

			for path, content := range files {
				require.NoError(t, vfs.WriteFile(v.FS, path, []byte(content), 0o644))
			}

			ctx, pctx := newDependentParsingContext(t, appConfigPath, tc.command)
			ctx = config.WithCaches(ctx)
			pctx.OriginalTerragruntConfigPath = appConfigPath
			require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OutputsBlock))

			cfg, err := new(config.Store).ResolveRunConfig(ctx, createLogger(), v, pctx, appConfigPath)
			tc.check(t, cfg, err)
			assert.Empty(t, recorder.outputInvocations(), "a missing state reads no output from OpenTofu")
		})
	}
}
