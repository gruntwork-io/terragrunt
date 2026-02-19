package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/puzpuzpuz/xsync/v3"

	"github.com/gruntwork-io/terragrunt/internal/cloner"
	enginecfg "github.com/gruntwork-io/terragrunt/internal/engine/config"
	"github.com/gruntwork-io/terragrunt/internal/errorconfig"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
)

const (
	defaultTFDataDir   = ".terraform"
	defaultSignalsFile = "error-signals.json"
)

// Options contains the configuration needed by run.Run and its helpers.
// This is a focused subset of options.TerragruntOptions.
type Options struct {
	Writer                       io.Writer
	ErrWriter                    io.Writer
	HookData                     any
	TerraformCliArgs             *iacargs.IacArgs
	Engine                       *enginecfg.Options
	Errors                       *errorconfig.Config
	FeatureFlags                 *xsync.MapOf[string, string]
	Telemetry                    *telemetry.Options
	SourceMap                    map[string]string
	Env                          map[string]string
	EngineLogLevel               string
	DownloadDir                  string
	RootWorkingDir               string
	TerragruntConfigPath         string
	OriginalTerragruntConfigPath string
	WorkingDir                   string
	EngineCachePath              string
	TofuImplementation           tfimpl.Type
	TerraformCommand             string
	OriginalTerraformCommand     string
	Source                       string
	TFPath                       string
	AuthProviderCmd              string
	ProviderCacheToken           string
	OriginalIAMRoleOptions       iam.RoleOptions
	IAMRoleOptions               iam.RoleOptions
	StrictControls               strict.Controls
	ProviderCacheRegistryNames   []string
	Experiments                  experiment.Experiments
	VersionManagerFileName       []string
	MaxFoldersToCheck            int
	AutoRetry                    bool
	EngineSkipChecksumCheck      bool
	Debug                        bool
	AutoInit                     bool
	Headless                     bool
	BackendBootstrap             bool
	NoEngine                     bool
	LogShowAbsPaths              bool
	LogDisableErrorSummary       bool
	NonInteractive               bool
	FailIfBucketCreationRequired bool
	DisableBucketUpdate          bool
	CheckDependentUnits          bool
	SkipOutput                   bool
	SourceUpdate                 bool
	JSONLogFormat                bool
	ForwardTFStdout              bool
}

// Clone performs a deep copy of Options.
func (o *Options) Clone() *Options {
	return cloner.Clone(o)
}

// CloneWithConfigPath creates a copy of Options with updated config path and working directory.
func (o *Options) CloneWithConfigPath(l log.Logger, configPath string) (log.Logger, *Options, error) {
	newOpts := o.Clone()

	configPath = util.CleanPath(configPath)
	if !filepath.IsAbs(configPath) {
		absConfigPath, err := filepath.Abs(configPath)
		if err != nil {
			return l, nil, err
		}

		configPath = util.CleanPath(absConfigPath)
	}

	workingDir := filepath.Dir(configPath)

	if workingDir != o.WorkingDir {
		l = l.WithField(placeholders.WorkDirKeyName, workingDir)
	}

	newOpts.TerragruntConfigPath = configPath
	newOpts.WorkingDir = workingDir

	return l, newOpts, nil
}

// InsertTerraformCliArgs inserts the given args after the terraform command argument.
func (o *Options) InsertTerraformCliArgs(argsToInsert ...string) {
	if o.TerraformCliArgs == nil {
		o.TerraformCliArgs = iacargs.New()
	}

	parsed := iacargs.New(argsToInsert...)

	o.TerraformCliArgs.InsertFlag(0, parsed.Flags...)

	// Handle command field
	switch {
	case o.TerraformCliArgs.Command == "":
		o.TerraformCliArgs.Command = parsed.Command
	case parsed.Command == "" || parsed.Command == o.TerraformCliArgs.Command:
		// no-op
	case iacargs.IsKnownSubCommand(parsed.Command):
		o.TerraformCliArgs.SubCommand = []string{parsed.Command}
	default:
		o.TerraformCliArgs.InsertArguments(0, parsed.Command)
	}

	if len(parsed.SubCommand) > 0 {
		o.TerraformCliArgs.SubCommand = parsed.SubCommand
	}

	o.TerraformCliArgs.InsertArguments(0, parsed.Arguments...)
}

// AppendTerraformCliArgs appends the given args after the current TerraformCliArgs.
func (o *Options) AppendTerraformCliArgs(argsToAppend ...string) {
	if o.TerraformCliArgs == nil {
		o.TerraformCliArgs = iacargs.New()
	}

	parsed := iacargs.New(argsToAppend...)

	o.TerraformCliArgs.AppendFlag(parsed.Flags...)

	if parsed.Command != "" {
		o.TerraformCliArgs.AppendArgument(parsed.Command)
	}

	o.TerraformCliArgs.AppendArgument(parsed.Arguments...)

	if len(parsed.SubCommand) > 0 {
		o.TerraformCliArgs.SubCommand = parsed.SubCommand
	}
}

// TerraformDataDir returns Terraform data directory (.terraform by default, overridden by $TF_DATA_DIR envvar)
func (o *Options) TerraformDataDir() string {
	if tfDataDir, ok := o.Env["TF_DATA_DIR"]; ok {
		return tfDataDir
	}

	return defaultTFDataDir
}

// DataDir returns the Terraform data directory prepended with the working directory path.
func (o *Options) DataDir() string {
	tfDataDir := o.TerraformDataDir()
	if filepath.IsAbs(tfDataDir) {
		return tfDataDir
	}

	return filepath.Join(o.WorkingDir, tfDataDir)
}

// shellRunOptions builds a *shell.RunOptions from this Options.
func (o *Options) shellRunOptions() *shell.RunOptions {
	return &shell.RunOptions{
		WorkingDir:              o.WorkingDir,
		Writer:                  o.Writer,
		ErrWriter:               o.ErrWriter,
		Env:                     o.Env,
		TFPath:                  o.TFPath,
		Engine:                  o.Engine,
		Experiments:             o.Experiments,
		NoEngine:                o.NoEngine,
		Telemetry:               o.Telemetry,
		RootWorkingDir:          o.RootWorkingDir,
		LogShowAbsPaths:         o.LogShowAbsPaths,
		LogDisableErrorSummary:  o.LogDisableErrorSummary,
		Headless:                o.Headless,
		ForwardTFStdout:         o.ForwardTFStdout,
		EngineCachePath:         o.EngineCachePath,
		EngineLogLevel:          o.EngineLogLevel,
		EngineSkipChecksumCheck: o.EngineSkipChecksumCheck,
	}
}

// tfRunOptions builds a *tf.RunOptions from this Options.
func (o *Options) tfRunOptions() *tf.RunOptions {
	return &tf.RunOptions{
		ForwardTFStdout:              o.ForwardTFStdout,
		Writer:                       o.Writer,
		ErrWriter:                    o.ErrWriter,
		TFPath:                       o.TFPath,
		JSONLogFormat:                o.JSONLogFormat,
		Headless:                     o.Headless,
		OriginalTerragruntConfigPath: o.OriginalTerragruntConfigPath,
		ShellRunOpts:                 o.shellRunOptions(),
		HookData:                     o.HookData,
	}
}

// remoteStateOpts builds a *remotestate.Options from this Options.
func (o *Options) remoteStateOpts() *remotestate.Options {
	return &remotestate.Options{
		Options: backend.Options{
			NonInteractive:               o.NonInteractive,
			FailIfBucketCreationRequired: o.FailIfBucketCreationRequired,
			ErrWriter:                    o.ErrWriter,
			Env:                          o.Env,
			IAMRoleOptions:               o.IAMRoleOptions,
		},
		DisableBucketUpdate: o.DisableBucketUpdate,
		TFRunOpts:           o.tfRunOptions(),
	}
}

// RunWithErrorHandling runs the given operation and handles errors according to the configuration.
func (o *Options) RunWithErrorHandling(
	ctx context.Context,
	l log.Logger,
	r *report.Report,
	operation func() error,
) error {
	if o.Errors == nil {
		return operation()
	}

	currentAttempt := 1

	reportWorkingDir := o.WorkingDir
	if o.OriginalTerragruntConfigPath != "" {
		reportWorkingDir = filepath.Dir(o.OriginalTerragruntConfigPath)
	}

	reportDir, err := filepath.Abs(reportWorkingDir)
	if err != nil {
		return err
	}

	reportDir = util.CleanPath(reportDir)

	for {
		err := operation()
		if err == nil {
			return nil
		}

		action, recoveryErr := o.Errors.AttemptErrorRecovery(l, err, currentAttempt)
		if recoveryErr != nil {
			var maxAttemptsReachedError *errorconfig.MaxAttemptsReachedError
			if errors.As(recoveryErr, &maxAttemptsReachedError) {
				return maxAttemptsReachedError
			}

			return fmt.Errorf("encountered error while attempting error recovery: %w", recoveryErr)
		}

		if action == nil {
			return err
		}

		if action.ShouldIgnore {
			l.Warnf("Ignoring error, reason: %s", action.IgnoreMessage)

			if len(action.IgnoreSignals) > 0 {
				if err := o.handleIgnoreSignals(l, action.IgnoreSignals); err != nil {
					return err
				}
			}

			run, err := r.EnsureRun(l, reportDir)
			if err != nil {
				return err
			}

			if err := r.EndRun(
				l,
				run.Path,
				report.WithResult(report.ResultSucceeded),
				report.WithReason(report.ReasonErrorIgnored),
				report.WithCauseIgnoreBlock(action.IgnoreBlockName),
			); err != nil {
				return err
			}

			return nil
		}

		if action.ShouldRetry {
			if !o.AutoRetry {
				return err
			}

			l.Warnf(
				"Encountered retryable error: %s\nAttempt %d of %d. Waiting %d second(s) before retrying...",
				action.RetryBlockName,
				currentAttempt,
				action.RetryAttempts,
				action.RetrySleepSecs,
			)

			run, err := r.EnsureRun(l, reportDir)
			if err != nil {
				return err
			}

			if err := r.EndRun(
				l,
				run.Path,
				report.WithResult(report.ResultSucceeded),
				report.WithReason(report.ReasonRetrySucceeded),
				report.WithCauseRetryBlock(action.RetryBlockName),
			); err != nil {
				return err
			}

			select {
			case <-time.After(time.Duration(action.RetrySleepSecs) * time.Second):
			case <-ctx.Done():
				return errors.New(ctx.Err())
			}

			currentAttempt++

			continue
		}

		return err
	}
}

func (o *Options) handleIgnoreSignals(l log.Logger, signals map[string]any) error {
	signalsFile := filepath.Join(o.WorkingDir, defaultSignalsFile)

	signalsJSON, err := json.MarshalIndent(signals, "", "  ")
	if err != nil {
		return err
	}

	const ownerPerms = 0644

	l.Warnf("Writing error signals to %s", signalsFile)

	if err := os.WriteFile(signalsFile, signalsJSON, ownerPerms); err != nil {
		return fmt.Errorf("failed to write signals file %s: %w", signalsFile, err)
	}

	return nil
}
