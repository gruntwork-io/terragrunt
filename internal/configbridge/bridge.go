// Package configbridge provides an adapter between *options.TerragruntOptions
// and *config.ParsingContext, allowing callers that have TerragruntOptions to
// invoke pkg/config functions without config needing to import pkg/options.
package configbridge

import (
	"context"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// NewParsingContext creates a config.ParsingContext populated from TerragruntOptions.
func NewParsingContext(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (context.Context, *config.ParsingContext) {
	ctx, pctx := config.NewParsingContext(ctx, l, opts.StrictControls)
	populateFromOpts(pctx, opts)

	pctx.OutputRunFunc = func(ctx context.Context, l log.Logger, currentPctx *config.ParsingContext, stdoutWriter io.Writer, runCfg *runcfg.RunConfig, credsGetter *creds.Getter) error {
		runOpts := optsFromParsingContext(currentPctx)
		runOpts.Writer = stdoutWriter
		runOpts.ForwardTFStdout = false
		runOpts.JSONLogFormat = false

		return run.Run(ctx, l, runOpts, report.NewReport(), runCfg, credsGetter)
	}

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

// optsFromParsingContext constructs a *options.TerragruntOptions from ParsingContext flat fields.
// Used by the OutputRunFunc callback to bridge back to run.Run which requires opts.
func optsFromParsingContext(pctx *config.ParsingContext) *options.TerragruntOptions {
	return &options.TerragruntOptions{
		TerragruntConfigPath:         pctx.TerragruntConfigPath,
		OriginalTerragruntConfigPath: pctx.OriginalTerragruntConfigPath,
		WorkingDir:                   pctx.WorkingDir,
		RootWorkingDir:               pctx.RootWorkingDir,
		DownloadDir:                  pctx.DownloadDir,
		Source:                       pctx.Source,
		SourceMap:                    pctx.SourceMap,
		TerraformCommand:             pctx.TerraformCommand,
		OriginalTerraformCommand:     pctx.OriginalTerraformCommand,
		TerraformCliArgs:             pctx.TerraformCliArgs,
		Writer:                       pctx.Writer,
		ErrWriter:                    pctx.ErrWriter,
		Env:                          pctx.Env,
		IAMRoleOptions:               pctx.IAMRoleOptions,
		OriginalIAMRoleOptions:       pctx.OriginalIAMRoleOptions,
		Experiments:                  pctx.Experiments,
		StrictControls:               pctx.StrictControls,
		FeatureFlags:                 pctx.FeatureFlags,
		Engine:                       pctx.Engine,
		LogShowAbsPaths:              pctx.LogShowAbsPaths,
		AuthProviderCmd:              pctx.AuthProviderCmd,
		TFPath:                       pctx.TFPath,
		Debug:                        pctx.Debug,
		AutoInit:                     pctx.AutoInit,
		BackendBootstrap:             pctx.BackendBootstrap,
		TofuImplementation:           pctx.TofuImplementation,
		Telemetry:                    pctx.Telemetry,
		NoEngine:                     pctx.NoEngine,
		Headless:                     pctx.Headless,
		LogDisableErrorSummary:       pctx.LogDisableErrorSummary,
	}
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
