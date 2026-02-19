// Package configbridge provides an adapter between *options.TerragruntOptions
// and *config.ParsingContext, allowing callers that have TerragruntOptions to
// invoke pkg/config functions without config needing to import pkg/options.
package configbridge

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// NewParsingContext creates a config.ParsingContext populated from TerragruntOptions.
func NewParsingContext(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (context.Context, *config.ParsingContext) {
	ctx, pctx := config.NewParsingContext(ctx, l, opts.StrictControls)
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
	pctx.Writer = opts.Writer
	pctx.ErrWriter = opts.ErrWriter
	pctx.Env = opts.Env
	pctx.IAMRoleOptions = opts.IAMRoleOptions
	pctx.OriginalIAMRoleOptions = opts.OriginalIAMRoleOptions
	pctx.UsePartialParseConfigCache = opts.UsePartialParseConfigCache
	pctx.MaxFoldersToCheck = opts.MaxFoldersToCheck
	pctx.NoDependencyFetchOutputFromState = opts.NoDependencyFetchOutputFromState
	pctx.SkipOutput = opts.SkipOutput
	pctx.TFPathExplicitlySet = opts.TFPathExplicitlySet
	pctx.LogShowAbsPaths = opts.LogShowAbsPaths
	pctx.AuthProviderCmd = opts.AuthProviderCmd
	pctx.Engine = opts.Engine
	pctx.TFPath = opts.TFPath
	pctx.TofuImplementation = opts.TofuImplementation
	pctx.ForwardTFStdout = opts.ForwardTFStdout
	pctx.JSONLogFormat = opts.JSONLogFormat
	pctx.Debug = opts.Debug
	pctx.AutoInit = opts.AutoInit
	pctx.Headless = opts.Headless
	pctx.BackendBootstrap = opts.BackendBootstrap
	pctx.NoEngine = opts.NoEngine
	pctx.CheckDependentUnits = opts.CheckDependentUnits
	pctx.LogDisableErrorSummary = opts.LogDisableErrorSummary
	pctx.Telemetry = opts.Telemetry
	pctx.NoStackValidate = opts.NoStackValidate
	pctx.ScaffoldRootFileName = opts.ScaffoldRootFileName
	pctx.TerragruntStackConfigPath = opts.TerragruntStackConfigPath
}

// ShellRunOptsFromPctx builds a *shell.RunOptions from ParsingContext flat fields.
// Exported so configbridge callbacks and external callers can use it.
func ShellRunOptsFromPctx(pctx *config.ParsingContext) *shell.RunOptions {
	return &shell.RunOptions{
		WorkingDir:             pctx.WorkingDir,
		Writer:                 pctx.Writer,
		ErrWriter:              pctx.ErrWriter,
		Env:                    pctx.Env,
		TFPath:                 pctx.TFPath,
		Engine:                 pctx.Engine,
		Experiments:            pctx.Experiments,
		NoEngine:               pctx.NoEngine,
		Telemetry:              pctx.Telemetry,
		RootWorkingDir:         pctx.RootWorkingDir,
		LogShowAbsPaths:        pctx.LogShowAbsPaths,
		LogDisableErrorSummary: pctx.LogDisableErrorSummary,
	}
}

// NewCredsProvider creates an externalcmd credentials provider from ParsingContext fields.
func NewCredsProvider(l log.Logger, pctx *config.ParsingContext) providers.Provider {
	return externalcmd.NewProvider(l, pctx.AuthProviderCmd, ShellRunOptsFromPctx(pctx))
}

// TFRunOptsFromOpts constructs tf.RunOptions from TerragruntOptions.
func TFRunOptsFromOpts(opts *options.TerragruntOptions) *tf.RunOptions {
	return &tf.RunOptions{
		ForwardTFStdout:              opts.ForwardTFStdout,
		Writer:                       opts.Writer,
		ErrWriter:                    opts.ErrWriter,
		TFPath:                       opts.TFPath,
		JSONLogFormat:                opts.JSONLogFormat,
		Headless:                     opts.Headless,
		OriginalTerragruntConfigPath: opts.OriginalTerragruntConfigPath,
		ShellRunOpts:                 shell.RunOptionsFromOpts(opts),
		HookData:                     opts,
	}
}
