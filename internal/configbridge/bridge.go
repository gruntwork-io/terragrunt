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
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// NewParsingContext creates a config.ParsingContext populated from TerragruntOptions.
func NewParsingContext(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
) (context.Context, *config.ParsingContext) {
	ctx, pctx := config.NewParsingContext(ctx, l, config.WithStrictControls(opts.StrictControls))
	populateFromOpts(pctx, opts)

	return ctx, pctx
}

// populateFromOpts copies fields from TerragruntOptions into ParsingContext flat fields.
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
	pctx.Writers = opts.Writers
	pctx.Env = opts.Env
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
}

// ShellRunOptsFromOpts constructs shell.ShellOptions from TerragruntOptions.
func ShellRunOptsFromOpts(opts *options.TerragruntOptions) *shell.ShellOptions {
	return shell.NewShellOptions().
		WithWorkingDir(opts.WorkingDir).
		WithEnv(opts.Env).
		WithWriters(opts.Writers).
		WithTelemetry(opts.Telemetry).
		WithEngine(opts.EngineConfig, opts.EngineOptions).
		WithTFPath(opts.TFPath).
		WithRootWorkingDir(opts.RootWorkingDir).
		WithExperiments(opts.Experiments).
		WithHeadless(opts.Headless).
		WithForwardTFStdout(opts.ForwardTFStdout)
}

// BackendOptsFromOpts constructs backend.Options from TerragruntOptions.
func BackendOptsFromOpts(opts *options.TerragruntOptions) *backend.Options {
	return &backend.Options{
		Writers:                      opts.Writers,
		Env:                          opts.Env,
		IAMRoleOptions:               opts.IAMRoleOptions,
		NonInteractive:               opts.NonInteractive,
		FailIfBucketCreationRequired: opts.FailIfBucketCreationRequired,
	}
}

// RemoteStateOptsFromOpts constructs remotestate.Options from TerragruntOptions.
func RemoteStateOptsFromOpts(opts *options.TerragruntOptions) *remotestate.Options {
	return &remotestate.Options{
		Options:             *BackendOptsFromOpts(opts),
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

// NewRunOptions creates a run.Options from TerragruntOptions.
// This replaces the former run.NewOptions(opts) function.
func NewRunOptions(opts *options.TerragruntOptions) *run.Options {
	return &run.Options{
		Writers:                      opts.Writers,
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
		Env:                          opts.Env,
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
	}
}
