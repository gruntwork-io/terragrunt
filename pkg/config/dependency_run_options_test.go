package config_test

import (
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunOptionsFromParsingContextCopiesEveryOption pins every field the
// dependency-output run carries over from the parsing context. A flag that
// stops being copied here keeps working for a direct run and silently stops
// applying to any unit reached through a `dependency` block.
func TestRunOptionsFromParsingContextCopiesEveryOption(t *testing.T) {
	t.Parallel()

	pctx := parsingContextWithDistinctValues(t)

	runOpts := config.RunOptionsFromParsingContext(pctx)
	require.NotNil(t, runOpts)

	assert.True(t, runOpts.LogShowAbsPaths)
	assert.True(t, runOpts.LogDisableErrorSummary)
	assert.Equal(t, pctx.TerragruntConfigPath, runOpts.TerragruntConfigPath)
	assert.Equal(t, pctx.OriginalTerragruntConfigPath, runOpts.OriginalTerragruntConfigPath)
	assert.Equal(t, pctx.WorkingDir, runOpts.UnitDir)
	assert.Equal(t, pctx.WorkingDir, runOpts.CacheDir)
	assert.Equal(t, pctx.RootWorkingDir, runOpts.RootWorkingDir)
	assert.Equal(t, pctx.DownloadDir, runOpts.DownloadDir)
	assert.Equal(t, pctx.Source, runOpts.Source)
	assert.Equal(t, pctx.SourceMap, runOpts.SourceMap)
	assert.Equal(t, pctx.TerraformCommand, runOpts.TerraformCommand)
	assert.Equal(t, pctx.OriginalTerraformCommand, runOpts.OriginalTerraformCommand)
	assert.Same(t, pctx.TerraformCliArgs, runOpts.TerraformCliArgs)
	assert.Equal(t, pctx.IAMRoleOptions, runOpts.IAMRoleOptions)
	assert.Equal(t, pctx.OriginalIAMRoleOptions, runOpts.OriginalIAMRoleOptions)
	assert.True(t, runOpts.Experiments.Evaluate(experiment.Stacks))
	assert.Equal(t, pctx.StrictControls, runOpts.StrictControls)
	assert.Equal(t, pctx.FeatureFlags, runOpts.FeatureFlags)
	assert.Same(t, pctx.EngineConfig, runOpts.EngineConfig)
	assert.Same(t, pctx.EngineOptions, runOpts.EngineOptions)
	assert.Equal(t, pctx.TFPath, runOpts.TFPath)
	assert.Equal(t, pctx.TofuImplementation, runOpts.TofuImplementation)
	assert.True(t, runOpts.Headless)
	assert.True(t, runOpts.Debug)
	assert.True(t, runOpts.AutoInit)
	assert.True(t, runOpts.BackendBootstrap)
	assert.Same(t, pctx.Telemetry, runOpts.Telemetry)
	assert.Equal(t, pctx.AuthProviderCmd, runOpts.AuthProviderCmd)
	assert.True(t, runOpts.NoCAS, "--no-cas must keep the CAS disabled for a dependency's source")
	assert.Equal(t, pctx.CASCloneDepth, runOpts.CASCloneDepth)
	assert.True(t, runOpts.CASOffline, "--cas-offline must still forbid fetching a dependency's source")
	assert.True(t, runOpts.CASRefresh)
	assert.Equal(t, pctx.CASProbeTTL, runOpts.CASProbeTTL)
}

// parsingContextWithDistinctValues returns a context whose every bridged
// field holds a value distinguishable from the zero value, so a dropped
// copy fails an assertion rather than passing on a coincidence.
func parsingContextWithDistinctValues(t *testing.T) *config.ParsingContext {
	t.Helper()

	_, pctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), venvtest.New())

	pctx.TerragruntConfigPath = "/dep/unit/terragrunt.hcl"
	pctx.OriginalTerragruntConfigPath = "/dep/original/terragrunt.hcl"
	pctx.WorkingDir = "/dep/unit"
	pctx.RootWorkingDir = "/dep"
	pctx.DownloadDir = "/dep/unit/.terragrunt-cache"
	pctx.Source = "github.com/acme/modules//vpc"
	pctx.SourceMap = map[string]string{"github.com/acme/modules": "/local/modules"}
	pctx.TerraformCommand = "output"
	pctx.OriginalTerraformCommand = "plan"
	pctx.TerraformCliArgs = iacargs.New("output", "-json")
	pctx.IAMRoleOptions = iam.RoleOptions{RoleARN: "arn:aws:iam::111111111111:role/dep"}
	pctx.OriginalIAMRoleOptions = iam.RoleOptions{RoleARN: "arn:aws:iam::111111111111:role/original"}
	pctx.Experiments = experiment.NewExperiments()
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.Stacks))
	pctx.StrictControls = controls.New()
	pctx.FeatureFlags = map[string]string{"region": "us-east-1"}
	pctx.EngineConfig = &engine.EngineConfig{Source: "engine-source"}
	pctx.EngineOptions = &engine.EngineOptions{}
	pctx.TFPath = "/usr/local/bin/tofu"
	pctx.TofuImplementation = tfimpl.OpenTofu
	pctx.Headless = true
	pctx.Debug = true
	pctx.AutoInit = true
	pctx.BackendBootstrap = true
	pctx.Telemetry = &telemetry.Options{}
	pctx.AuthProviderCmd = "/usr/local/bin/auth"
	pctx.NoCAS = true
	pctx.CASCloneDepth = 7
	pctx.CASOffline = true
	pctx.CASRefresh = true
	pctx.CASProbeTTL = 90 * time.Second
	pctx.LogShowAbsPaths = true
	pctx.LogDisableErrorSummary = true

	return pctx
}
