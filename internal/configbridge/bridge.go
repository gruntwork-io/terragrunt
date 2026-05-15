// Package configbridge provides an adapter between *options.TerragruntOptions
// and *config.ParsingContext, allowing callers that have TerragruntOptions to
// invoke pkg/config functions without config needing to import pkg/options.
package configbridge

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// NewParsingContext creates a config.ParsingContext populated from
// TerragruntOptions. Shell environment and writers travel separately via
// the venv threaded to leaf consumers; they are no longer mirrored here.
func NewParsingContext(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
) (context.Context, *config.ParsingContext) {
	ctx, pctx := config.NewParsingContext(ctx, l, config.WithStrictControls(opts.StrictControls))
	populateFromOpts(pctx, opts)

	return ctx, pctx
}

// ShellRunOptsFromOpts constructs shell.ShellOptions from TerragruntOptions.
// The shell-options struct no longer carries env or writers; callers pass
// those via the venv at invocation time.
func ShellRunOptsFromOpts(opts *options.TerragruntOptions) *shell.ShellOptions {
	s := shell.NewShellOptions().
		WithWorkingDir(opts.WorkingDir).
		WithTelemetry(opts.Telemetry).
		WithEngine(opts.EngineConfig, opts.EngineOptions).
		WithTFPath(opts.TFPath).
		WithRootWorkingDir(opts.RootWorkingDir).
		WithExperiments(opts.Experiments).
		WithHeadless(opts.Headless).
		WithForwardTFStdout(opts.ForwardTFStdout)
	s.LogShowAbsPaths = opts.LogShowAbsPaths
	s.LogDisableErrorSummary = opts.LogDisableErrorSummary

	return s
}

// BackendOptsFromOpts constructs backend.Options from TerragruntOptions and v.
// Env and Writers are populated from v so the Backend interface receives the
// venv data it needs at each invocation.
func BackendOptsFromOpts(v *venv.Venv, opts *options.TerragruntOptions) *backend.Options {
	return &backend.Options{
		Writers:                      v.Writers,
		Env:                          v.Env,
		IAMRoleOptions:               opts.IAMRoleOptions,
		NonInteractive:               opts.NonInteractive,
		FailIfBucketCreationRequired: opts.FailIfBucketCreationRequired,
		LogShowAbsPaths:              opts.LogShowAbsPaths,
		LogDisableErrorSummary:       opts.LogDisableErrorSummary,
	}
}

// RemoteStateOptsFromOpts constructs remotestate.Options from TerragruntOptions.
func RemoteStateOptsFromOpts(v *venv.Venv, opts *options.TerragruntOptions) *remotestate.Options {
	return &remotestate.Options{
		Options:             *BackendOptsFromOpts(v, opts),
		DisableBucketUpdate: opts.DisableBucketUpdate,
		TFRunOpts:           TFRunOptsFromOpts(opts),
	}
}

// TFRunOptsFromOpts constructs tf.TFOptions from TerragruntOptions.
func TFRunOptsFromOpts(opts *options.TerragruntOptions) *tf.TFOptions {
	return &tf.TFOptions{
		JSONLogFormat:                opts.JSONLogFormat,
		OriginalTerragruntConfigPath: opts.OriginalTerragruntConfigPath,
		TerragruntConfigPath:         opts.TerragruntConfigPath,
		TofuImplementation:           opts.TofuImplementation,
		TerraformCliArgs:             opts.TerraformCliArgs,
		ShellOptions:                 ShellRunOptsFromOpts(opts),
	}
}

// NewRunOptions creates a run.Options from TerragruntOptions. Env and
// writers are no longer mirrored onto the run.Options struct; callers
// pass them via the venv threaded into run.Run.
func NewRunOptions(opts *options.TerragruntOptions) *run.Options {
	return &run.Options{
		TerragruntConfigPath:         opts.TerragruntConfigPath,
		OriginalTerragruntConfigPath: opts.OriginalTerragruntConfigPath,
		WorkingDir:                   opts.WorkingDir,
		RootWorkingDir:               opts.RootWorkingDir,
		DownloadDir:                  opts.DownloadDir,
		TerraformCommand:             opts.TerraformCommand,
		OriginalTerraformCommand:     opts.OriginalTerraformCommand,
		TerraformCliArgs:             opts.TerraformCliArgs,
		Source:                       opts.Source,
		SourceMap:                    opts.SourceMap,
		IAMRoleOptions:               opts.IAMRoleOptions,
		OriginalIAMRoleOptions:       opts.OriginalIAMRoleOptions,
		EngineConfig:                 opts.EngineConfig,
		EngineOptions:                opts.EngineOptions,
		Errors:                       opts.Errors,
		Experiments:                  opts.Experiments,
		StrictControls:               opts.StrictControls,
		FeatureFlags:                 opts.FeatureFlags,
		TFPath:                       opts.TFPath,
		TofuImplementation:           opts.TofuImplementation,
		ForwardTFStdout:              opts.ForwardTFStdout,
		JSONLogFormat:                opts.JSONLogFormat,
		Headless:                     opts.Headless,
		NonInteractive:               opts.NonInteractive,
		Debug:                        opts.Debug,
		AutoInit:                     opts.AutoInit,
		AutoRetry:                    opts.AutoRetry,
		BackendBootstrap:             opts.BackendBootstrap,
		Telemetry:                    opts.Telemetry,
		AuthProviderCmd:              opts.AuthProviderCmd,
		MaxFoldersToCheck:            opts.MaxFoldersToCheck,
		FailIfBucketCreationRequired: opts.FailIfBucketCreationRequired,
		DisableBucketUpdate:          opts.DisableBucketUpdate,
		SourceUpdate:                 opts.SourceUpdate,
		CASCloneDepth:                opts.CASCloneDepth,
		NoCAS:                        opts.NoCAS,
		LogShowAbsPaths:              opts.LogShowAbsPaths,
		LogDisableErrorSummary:       opts.LogDisableErrorSummary,
	}
}

// populateFromOpts copies fields from TerragruntOptions into ParsingContext
// flat fields. It is colocated with [NewParsingContext] but kept private so
// callers go through the exported constructor.
func populateFromOpts(pctx *config.ParsingContext, opts *options.TerragruntOptions) {
	pctx.TerragruntConfigPath = opts.TerragruntConfigPath
	pctx.OriginalTerragruntConfigPath = opts.OriginalTerragruntConfigPath
	pctx.WorkingDir = opts.WorkingDir
	pctx.RootWorkingDir = opts.RootWorkingDir
	pctx.DownloadDir = opts.DownloadDir
	pctx.TerraformCommand = opts.TerraformCommand
	pctx.OriginalTerraformCommand = opts.OriginalTerraformCommand
	pctx.TerraformCliArgs = opts.TerraformCliArgs
	pctx.Source = opts.Source
	pctx.SourceMap = opts.SourceMap
	pctx.Experiments = opts.Experiments
	pctx.StrictControls = opts.StrictControls
	pctx.FeatureFlags = opts.FeatureFlags
	pctx.IAMRoleOptions = opts.IAMRoleOptions
	pctx.OriginalIAMRoleOptions = opts.OriginalIAMRoleOptions
	pctx.UsePartialParseConfigCache = opts.UsePartialParseConfigCache
	pctx.MaxFoldersToCheck = opts.MaxFoldersToCheck
	pctx.NoDependencyFetchOutputFromState = opts.NoDependencyFetchOutputFromState
	pctx.SkipOutput = opts.SkipOutput
	pctx.TFPathExplicitlySet = opts.TFPathExplicitlySet
	pctx.AuthProviderCmd = opts.AuthProviderCmd
	pctx.EngineConfig = opts.EngineConfig
	pctx.EngineOptions = opts.EngineOptions
	pctx.TFPath = opts.TFPath
	pctx.TofuImplementation = opts.TofuImplementation
	pctx.ForwardTFStdout = opts.ForwardTFStdout
	pctx.JSONLogFormat = opts.JSONLogFormat
	pctx.Debug = opts.Debug
	pctx.AutoInit = opts.AutoInit
	pctx.Headless = opts.Headless
	pctx.BackendBootstrap = opts.BackendBootstrap
	pctx.CheckDependentUnits = opts.CheckDependentUnits
	pctx.Telemetry = opts.Telemetry
	pctx.NoStackValidate = opts.NoStackValidate
	pctx.NoCAS = opts.NoCAS
	pctx.CASCloneDepth = opts.CASCloneDepth
	pctx.ScaffoldRootFileName = opts.ScaffoldRootFileName
	pctx.TerragruntStackConfigPath = opts.TerragruntStackConfigPath
	pctx.ProviderCacheOptions = opts.ProviderCacheOptions
	pctx.LogShowAbsPaths = opts.LogShowAbsPaths
	pctx.LogDisableErrorSummary = opts.LogDisableErrorSummary
}
