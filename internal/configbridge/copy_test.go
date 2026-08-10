package configbridge_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/errorconfig"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	pcoptions "github.com/gruntwork-io/terragrunt/internal/providercache/options"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewParsingContextCopiesEveryOption pins every bridged option onto the ParsingContext so no flag is dropped.
func TestNewParsingContextCopiesEveryOption(t *testing.T) {
	t.Parallel()

	opts := optionsWithDistinctValues(t)
	v := venvtest.New()

	_, pctx := configbridge.NewParsingContext(t.Context(), logger.CreateLogger(), v, opts)
	require.NotNil(t, pctx)

	assert.Same(t, v, pctx.Venv, "HCL helpers shell out and read files through the venv handed to the bridge")
	assert.Equal(t, opts.TerragruntConfigPath, pctx.TerragruntConfigPath)
	assert.Equal(t, opts.OriginalTerragruntConfigPath, pctx.OriginalTerragruntConfigPath)
	assert.Equal(t, opts.WorkingDir, pctx.WorkingDir)
	assert.Equal(t, opts.RootWorkingDir, pctx.RootWorkingDir)
	assert.Equal(t, opts.DownloadDir, pctx.DownloadDir)
	assert.Equal(t, opts.TerraformCommand, pctx.TerraformCommand)
	assert.Equal(t, opts.OriginalTerraformCommand, pctx.OriginalTerraformCommand)
	assert.Same(t, opts.TerraformCliArgs, pctx.TerraformCliArgs, "the copy must replace the args the constructor seeds")
	assert.Equal(t, opts.Source, pctx.Source)
	assert.Equal(t, opts.SourceMap, pctx.SourceMap)
	assert.True(t, pctx.Experiments.Evaluate(experiment.Stacks), "enabled experiments must reach config parsing")
	assert.Equal(t, opts.StrictControls, pctx.StrictControls)
	assert.True(t, pctx.LogShowAbsPaths)
	assert.True(t, pctx.LogDisableErrorSummary)
	assert.Equal(t, opts.IAMRoleOptions, pctx.IAMRoleOptions)
	assert.Equal(t, opts.OriginalIAMRoleOptions, pctx.OriginalIAMRoleOptions)
	assert.True(t, pctx.UsePartialParseConfigCache)
	assert.Equal(t, opts.MaxFoldersToCheck, pctx.MaxFoldersToCheck)
	assert.True(t, pctx.NoDependencyFetchOutputFromState)
	assert.True(t, pctx.SkipOutput)
	assert.True(t, pctx.TFPathExplicitlySet)
	assert.Equal(t, opts.AuthProviderCmd, pctx.AuthProviderCmd)
	assert.Same(t, opts.EngineConfig, pctx.EngineConfig)
	assert.Same(t, opts.EngineOptions, pctx.EngineOptions)
	assert.Equal(t, opts.TFPath, pctx.TFPath)
	assert.Equal(t, opts.TofuImplementation, pctx.TofuImplementation)
	assert.True(t, pctx.ForwardTFStdout)
	assert.True(t, pctx.JSONLogFormat)
	assert.True(t, pctx.Debug)
	assert.True(t, pctx.AutoInit)
	assert.True(t, pctx.Headless)
	assert.True(t, pctx.BackendBootstrap)
	assert.True(t, pctx.CheckDependentUnits)
	assert.Same(t, opts.Telemetry, pctx.Telemetry)
	assert.True(t, pctx.NoStackValidate)
	assert.True(t, pctx.NoCAS)
	assert.Equal(t, opts.CASCloneDepth, pctx.CASCloneDepth)
	assert.Equal(t, opts.ScaffoldRootFileName, pctx.ScaffoldRootFileName)
	assert.Equal(t, opts.TerragruntStackConfigPath, pctx.TerragruntStackConfigPath)
	assert.Equal(t, opts.ProviderCacheOptions, pctx.ProviderCacheOptions)

	require.NotNil(t, pctx.FeatureFlags)

	flag, ok := pctx.FeatureFlags.Load("region")
	require.True(t, ok, "feature flags supplied on the CLI must reach config parsing")
	assert.Equal(t, "us-east-1", flag)
}

// TestNewParsingContextPropagatesStrictControls pins that a control enabled on the options arrives enabled.
func TestNewParsingContextPropagatesStrictControls(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("terragrunt.hcl")
	require.NoError(t, err)
	require.NoError(t, opts.StrictControls.EnableControl(controls.BareInclude))

	_, pctx := configbridge.NewParsingContext(t.Context(), logger.CreateLogger(), venvtest.New(), opts)

	ctrl := pctx.StrictControls.Find(controls.BareInclude)
	require.NotNil(t, ctrl, "strict controls must reach the parsing context")
	assert.True(t, ctrl.GetEnabled(), "a control enabled on the options must arrive enabled")
	assert.NotEmpty(t, pctx.ParserOptions, "parser options are built from the strict controls at construction")
}

// TestNewRunOptionsCopiesEveryOption pins every bridged option onto the run options so no flag is dropped.
func TestNewRunOptionsCopiesEveryOption(t *testing.T) {
	t.Parallel()

	opts := optionsWithDistinctValues(t)

	opts.ProfileDir = "/run/profiles"
	opts.NonInteractive = true
	opts.AutoRetry = true
	opts.FailIfBucketCreationRequired = true
	opts.DisableBucketUpdate = true
	opts.SourceUpdate = true
	opts.NoRunHooks = true
	opts.Errors = &errorconfig.Config{
		Retry: map[string]*errorconfig.RetryConfig{
			"transient": {Name: "transient", MaxAttempts: 3},
		},
	}

	runOpts := configbridge.NewRunOptions(opts)
	require.NotNil(t, runOpts)

	assert.True(t, runOpts.LogShowAbsPaths)
	assert.True(t, runOpts.LogDisableErrorSummary)
	assert.Equal(t, opts.TerragruntConfigPath, runOpts.TerragruntConfigPath)
	assert.Equal(t, opts.OriginalTerragruntConfigPath, runOpts.OriginalTerragruntConfigPath)
	assert.Equal(t, opts.WorkingDir, runOpts.UnitDir, "the unit dir is the working dir, not a separate option")
	assert.Equal(t, opts.WorkingDir, runOpts.CacheDir, "the cache dir is the working dir, not the download dir")
	assert.Equal(t, opts.RootWorkingDir, runOpts.RootWorkingDir)
	assert.Equal(t, opts.ProfileDir, runOpts.ProfileDir)
	assert.Equal(t, opts.DownloadDir, runOpts.DownloadDir)
	assert.Equal(t, opts.TerraformCommand, runOpts.TerraformCommand)
	assert.Equal(t, opts.OriginalTerraformCommand, runOpts.OriginalTerraformCommand)
	assert.Same(t, opts.TerraformCliArgs, runOpts.TerraformCliArgs)
	assert.Equal(t, opts.Source, runOpts.Source)
	assert.Equal(t, opts.SourceMap, runOpts.SourceMap)
	assert.Equal(t, opts.IAMRoleOptions, runOpts.IAMRoleOptions)
	assert.Equal(t, opts.OriginalIAMRoleOptions, runOpts.OriginalIAMRoleOptions)
	assert.Same(t, opts.EngineConfig, runOpts.EngineConfig)
	assert.Same(t, opts.EngineOptions, runOpts.EngineOptions)
	assert.Same(t, opts.Errors, runOpts.Errors)
	assert.True(t, runOpts.Experiments.Evaluate(experiment.Stacks), "enabled experiments must reach the runner")
	assert.Equal(t, opts.StrictControls, runOpts.StrictControls)
	assert.Equal(t, opts.TFPath, runOpts.TFPath)
	assert.Equal(t, opts.TofuImplementation, runOpts.TofuImplementation)
	assert.True(t, runOpts.ForwardTFStdout)
	assert.True(t, runOpts.JSONLogFormat)
	assert.True(t, runOpts.Headless)
	assert.True(t, runOpts.NonInteractive)
	assert.True(t, runOpts.Debug)
	assert.True(t, runOpts.AutoInit)
	assert.True(t, runOpts.AutoRetry)
	assert.True(t, runOpts.BackendBootstrap)
	assert.Same(t, opts.Telemetry, runOpts.Telemetry)
	assert.Equal(t, opts.AuthProviderCmd, runOpts.AuthProviderCmd)
	assert.Equal(t, opts.MaxFoldersToCheck, runOpts.MaxFoldersToCheck)
	assert.True(t, runOpts.FailIfBucketCreationRequired)
	assert.True(t, runOpts.DisableBucketUpdate)
	assert.True(t, runOpts.SourceUpdate)
	assert.Equal(t, opts.CASCloneDepth, runOpts.CASCloneDepth)
	assert.True(t, runOpts.NoCAS)
	assert.True(t, runOpts.NoHooks, "NoHooks is fed by NoRunHooks, so before/after hooks stay disabled when asked")

	require.NotNil(t, runOpts.FeatureFlags)

	flag, ok := runOpts.FeatureFlags.Load("region")
	require.True(t, ok, "feature flags supplied on the CLI must reach the runner")
	assert.Equal(t, "us-east-1", flag)
}

// TestNewRunOptionsLeavesHooksEnabledByDefault pins that hooks stay enabled unless the user asks otherwise.
func TestNewRunOptionsLeavesHooksEnabledByDefault(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("terragrunt.hcl")
	require.NoError(t, err)

	runOpts := configbridge.NewRunOptions(opts)

	assert.False(t, runOpts.NoHooks, "hooks run unless the user asks for --no-run-hooks")
}

// TestShellRunOptsFromOptsCopiesEveryOption pins every option the shell needs to run OpenTofu/Terraform.
func TestShellRunOptsFromOptsCopiesEveryOption(t *testing.T) {
	t.Parallel()

	opts := optionsWithDistinctValues(t)

	shellOpts := configbridge.ShellRunOptsFromOpts(opts)
	require.NotNil(t, shellOpts)

	assert.Equal(t, opts.WorkingDir, shellOpts.WorkingDir, "commands run in the working dir of the unit")
	assert.Equal(t, opts.RootWorkingDir, shellOpts.RootWorkingDir)
	assert.Equal(t, opts.TFPath, shellOpts.TFPath)
	assert.Same(t, opts.Telemetry, shellOpts.Telemetry, "the copy must replace the telemetry the constructor seeds")
	assert.Same(t, opts.EngineConfig, shellOpts.EngineConfig)
	assert.Same(t, opts.EngineOptions, shellOpts.EngineOptions)
	assert.True(t, shellOpts.Experiments.Evaluate(experiment.Stacks), "enabled experiments must reach the shell")
	assert.True(t, shellOpts.Headless)
	assert.True(t, shellOpts.ForwardTFStdout)
	assert.True(t, shellOpts.LogShowAbsPaths)
	assert.True(t, shellOpts.LogDisableErrorSummary)
}

// TestTFRunOptsFromOptsCopiesEveryOption pins the options that state migration pull and push run with.
func TestTFRunOptsFromOptsCopiesEveryOption(t *testing.T) {
	t.Parallel()

	opts := optionsWithDistinctValues(t)

	tfOpts := configbridge.TFRunOptsFromOpts(opts)
	require.NotNil(t, tfOpts)

	assert.True(t, tfOpts.JSONLogFormat)
	assert.Equal(t, opts.TerragruntConfigPath, tfOpts.TerragruntConfigPath)
	assert.Equal(t, opts.OriginalTerragruntConfigPath, tfOpts.OriginalTerragruntConfigPath)
	assert.Equal(t, opts.TofuImplementation, tfOpts.TofuImplementation)
	assert.Same(t, opts.TerraformCliArgs, tfOpts.TerraformCliArgs)

	require.NotNil(t, tfOpts.ShellOptions, "tf commands shell out, so the shell options must be threaded through")
	assert.Equal(t, opts.WorkingDir, tfOpts.ShellOptions.WorkingDir)
}

// TestStackFuncFactoryScopesFunctionsToStackDir pins that get_working_dir resolves to the stack dir, not the unit.
func TestStackFuncFactoryScopesFunctionsToStackDir(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("/mem/root/live/terragrunt.hcl")
	require.NoError(t, err)

	opts.WorkingDir = "/mem/root/live"
	opts.RootWorkingDir = "/mem/root"

	funcsFor := configbridge.StackFuncFactory(t.Context(), logger.CreateLogger(), venvtest.New(), opts)

	testCases := []struct {
		name     string
		stackDir string
	}{
		{
			name:     "first stack dir",
			stackDir: "/mem/root/live/stack-a",
		},
		{
			name:     "second stack dir",
			stackDir: "/mem/root/live/stack-b",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			funcs, err := funcsFor(tc.stackDir)
			require.NoError(t, err)
			assert.NotEmpty(t, funcs, "stack files are evaluated with this map, so an empty one breaks every function")

			workingDir, ok := funcs[config.FuncNameGetWorkingDir]
			require.True(t, ok, "the factory must inject the dir-sensitive get_working_dir override")

			got, err := workingDir.Call(nil)
			require.NoError(t, err)
			assert.NotEqual(t, opts.WorkingDir, got.AsString(), "the unit working dir must not leak into a stack file")
			assert.Equal(t, tc.stackDir, got.AsString(), "each call is scoped to the stack dir it was given")
		})
	}
}

// optionsWithDistinctValues returns options whose every bridged field holds a recognizable non-zero value.
func optionsWithDistinctValues(t *testing.T) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest("terragrunt.hcl")
	require.NoError(t, err)
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.Stacks))

	opts.FeatureFlags.Store("region", "us-east-1")

	opts.TerragruntConfigPath = "/copy/unit/terragrunt.hcl"
	opts.OriginalTerragruntConfigPath = "/copy/original/terragrunt.hcl"
	opts.WorkingDir = "/copy/working"
	opts.RootWorkingDir = "/copy/root"
	opts.DownloadDir = "/copy/download"
	opts.TerraformCommand = "apply"
	opts.OriginalTerraformCommand = "plan"
	opts.TerraformCliArgs = iacargs.New("plan", "-input=false")
	opts.Source = "github.com/acme/units//vpc"
	opts.SourceMap = map[string]string{"github.com/acme/units": "/copy/local/units"}
	opts.AuthProviderCmd = "/copy/bin/auth-provider"
	opts.TFPath = "/copy/bin/tofu"
	opts.TofuImplementation = tfimpl.OpenTofu
	opts.ScaffoldRootFileName = "copy-root.hcl"
	opts.TerragruntStackConfigPath = "/copy/unit/terragrunt.stack.hcl"
	opts.MaxFoldersToCheck = 42
	opts.CASCloneDepth = 7
	opts.IAMRoleOptions = iam.RoleOptions{
		RoleARN:               "arn:aws:iam::111111111111:role/resolved",
		AssumeRoleSessionName: "resolved-session",
		AssumeRoleDuration:    3600,
	}
	opts.OriginalIAMRoleOptions = iam.RoleOptions{
		RoleARN:               "arn:aws:iam::222222222222:role/original",
		AssumeRoleSessionName: "original-session",
		AssumeRoleDuration:    900,
	}
	opts.EngineConfig = &engine.EngineConfig{
		Source:  "github.com/acme/engine",
		Version: "0.1.2",
		Type:    "rpc",
	}
	opts.EngineOptions = &engine.EngineOptions{
		CachePath: "/copy/engine-cache",
		LogLevel:  "debug",
	}
	opts.Telemetry = &telemetry.Options{
		TraceExporter:             "otlpHttp",
		TraceExporterHTTPEndpoint: "http://127.0.0.1:4318",
	}
	opts.ProviderCacheOptions = pcoptions.ProviderCacheOptions{
		Dir:           "/copy/provider-cache",
		Hostname:      "127.0.0.1",
		Token:         "copy-token",
		RegistryNames: []string{"registry.opentofu.org"},
		Port:          8899,
		Enabled:       true,
	}

	opts.LogShowAbsPaths = true
	opts.LogDisableErrorSummary = true
	opts.UsePartialParseConfigCache = true
	opts.NoDependencyFetchOutputFromState = true
	opts.SkipOutput = true
	opts.TFPathExplicitlySet = true
	opts.ForwardTFStdout = true
	opts.JSONLogFormat = true
	opts.Debug = true
	opts.AutoInit = true
	opts.Headless = true
	opts.BackendBootstrap = true
	opts.CheckDependentUnits = true
	opts.NoStackValidate = true
	opts.NoCAS = true

	return opts
}
