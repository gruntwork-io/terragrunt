package types

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-version"
	"github.com/puzpuzpuz/xsync/v3"
)

// RunnerOptions contains the options needed by the runner package.
// This is a runner-specific alternative to the global options.TerragruntOptions,
// containing only the fields needed by the runner for executing Terraform/OpenTofu.
type RunnerOptions struct {
	// I/O
	Writer    io.Writer
	ErrWriter io.Writer

	// Terraform/OpenTofu execution
	TerraformCommand        string
	TerraformCliArgs        cli.Args
	TerraformImplementation options.TerraformImplementationType
	TerraformVersion        *version.Version
	TFPath                  string
	TFPathExplicitlySet     bool

	// Paths
	WorkingDir           string
	TerragruntConfigPath string
	RootWorkingDir       string
	DownloadDir          string

	// Output configuration
	OutputFolder     string
	JSONOutputFolder string

	// Feature flags and configuration
	FeatureFlags           *xsync.MapOf[string, string]
	Experiments            experiment.Experiments
	Engine                 *options.EngineOptions
	EngineEnabled          bool
	Errors                 *options.ErrorsConfig
	Telemetry              *telemetry.Options
	LogDisableErrorSummary bool

	// IAM and environment
	IAMRoleOptions         options.IAMRoleOptions
	OriginalIAMRoleOptions options.IAMRoleOptions
	Env                    map[string]string

	// Behavior flags
	AutoInit                    bool
	AutoRetry                   bool
	BackendBootstrap            bool
	CheckDependentModules       bool
	Debug                       bool
	Headless                    bool
	IgnoreDependencyErrors      bool
	IncludeExternalDependencies bool
	NonInteractive              bool
	SourceUpdate                bool
	RunAllAutoApprove           bool
	FailFast                    bool
	IgnoreDependencyOrder       bool
	ForwardTFStdout             bool
	JSONLogFormat               bool

	// Execution configuration
	Parallelism                  int
	OriginalTerragruntConfigPath string

	// Source configuration
	Source                 string
	VersionManagerFileName []string
	TerragruntVersion      *version.Version

	// RunTerragrunt is the function to execute Terragrunt with the given options.
	// This allows the runner to call back into the main execution flow.
	RunTerragrunt func(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, r *report.Report) error
}

// FromTerragruntOptions extracts RunnerOptions from TerragruntOptions.
// This creates a runner-specific copy with only the fields needed by the runner package.
func FromTerragruntOptions(opts *options.TerragruntOptions) *RunnerOptions {
	if opts == nil {
		return nil
	}

	return &RunnerOptions{
		// I/O
		Writer:    opts.Writer,
		ErrWriter: opts.ErrWriter,

		// Terraform/OpenTofu execution
		TerraformCommand:        opts.TerraformCommand,
		TerraformCliArgs:        opts.TerraformCliArgs,
		TerraformImplementation: opts.TerraformImplementation,
		TerraformVersion:        opts.TerraformVersion,
		TFPath:                  opts.TFPath,
		TFPathExplicitlySet:     opts.TFPathExplicitlySet,

		// Paths
		WorkingDir:           opts.WorkingDir,
		TerragruntConfigPath: opts.TerragruntConfigPath,
		RootWorkingDir:       opts.RootWorkingDir,
		DownloadDir:          opts.DownloadDir,

		// Output configuration
		OutputFolder:     opts.OutputFolder,
		JSONOutputFolder: opts.JSONOutputFolder,

		// Feature flags and configuration
		FeatureFlags:           opts.FeatureFlags,
		Experiments:            opts.Experiments,
		Engine:                 opts.Engine,
		EngineEnabled:          opts.EngineEnabled,
		Errors:                 opts.Errors,
		Telemetry:              opts.Telemetry,
		LogDisableErrorSummary: opts.LogDisableErrorSummary,

		// IAM and environment
		IAMRoleOptions:         opts.IAMRoleOptions,
		OriginalIAMRoleOptions: opts.OriginalIAMRoleOptions,
		Env:                    opts.Env,

		// Behavior flags
		AutoInit:                    opts.AutoInit,
		AutoRetry:                   opts.AutoRetry,
		BackendBootstrap:            opts.BackendBootstrap,
		CheckDependentModules:       opts.CheckDependentModules,
		Debug:                       opts.Debug,
		Headless:                    opts.Headless,
		IgnoreDependencyErrors:      opts.IgnoreDependencyErrors,
		IncludeExternalDependencies: opts.IncludeExternalDependencies,
		NonInteractive:              opts.NonInteractive,
		SourceUpdate:                opts.SourceUpdate,
		RunAllAutoApprove:           opts.RunAllAutoApprove,
		FailFast:                    opts.FailFast,
		IgnoreDependencyOrder:       opts.IgnoreDependencyOrder,
		ForwardTFStdout:             opts.ForwardTFStdout,
		JSONLogFormat:               opts.JSONLogFormat,

		// Execution configuration
		Parallelism:                  opts.Parallelism,
		OriginalTerragruntConfigPath: opts.OriginalTerragruntConfigPath,

		// Source configuration
		Source:                 opts.Source,
		VersionManagerFileName: opts.VersionManagerFileName,
		TerragruntVersion:      opts.TerragruntVersion,

		// Function callbacks
		RunTerragrunt: opts.RunTerragrunt,
	}
}

// Clone creates a deep copy of RunnerOptions.
func (opts *RunnerOptions) Clone() *RunnerOptions {
	if opts == nil {
		return nil
	}

	// Deep clone using the cloner package
	newOpts := &RunnerOptions{}
	*newOpts = *opts

	// Deep clone slices and maps
	if opts.TerraformCliArgs != nil {
		newOpts.TerraformCliArgs = make([]string, len(opts.TerraformCliArgs))
		copy(newOpts.TerraformCliArgs, opts.TerraformCliArgs)
	}

	if opts.VersionManagerFileName != nil {
		newOpts.VersionManagerFileName = make([]string, len(opts.VersionManagerFileName))
		copy(newOpts.VersionManagerFileName, opts.VersionManagerFileName)
	}

	if opts.Env != nil {
		newOpts.Env = make(map[string]string, len(opts.Env))
		for k, v := range opts.Env {
			newOpts.Env[k] = v
		}
	}

	return newOpts
}

// CloneWithConfigPath creates a copy of RunnerOptions with a new config path and working directory.
// This is useful for creating options for a Terraform module in a different folder.
func (opts *RunnerOptions) CloneWithConfigPath(configPath string) (*RunnerOptions, error) {
	if opts == nil {
		return nil, nil
	}

	newOpts := opts.Clone()

	// Ensure configPath is absolute and normalized
	configPath = util.CleanPath(configPath)
	if !filepath.IsAbs(configPath) {
		absConfigPath, err := filepath.Abs(configPath)
		if err != nil {
			return nil, err
		}
		configPath = util.CleanPath(absConfigPath)
	}

	workingDir := filepath.Dir(configPath)

	newOpts.TerragruntConfigPath = configPath
	newOpts.WorkingDir = workingDir

	return newOpts, nil
}

// InsertTerraformCliArgs inserts the given args after the terraform command, but before remaining args.
// This handles special cases like planfile extraction and subcommands.
func (opts *RunnerOptions) InsertTerraformCliArgs(argsToInsert ...string) {
	planFile, restArgs := extractPlanFile(argsToInsert)

	commandLength := 1
	if len(opts.TerraformCliArgs) > 0 && util.ListContainsElement(options.TerraformCommandsWithSubcommand, opts.TerraformCliArgs[0]) {
		// Terraform commands with subcommands (e.g., "state list")
		commandLength = util.Min(2, len(opts.TerraformCliArgs))
	}

	// Options must be inserted after command but before other args
	var args []string
	args = append(args, opts.TerraformCliArgs[:commandLength]...)
	args = append(args, restArgs...)
	args = append(args, opts.TerraformCliArgs[commandLength:]...)

	// Append planfile at the end if extracted
	if planFile != nil {
		args = append(args, *planFile)
	}

	opts.TerraformCliArgs = args
}

// AppendTerraformCliArgs appends the given args to TerraformCliArgs.
func (opts *RunnerOptions) AppendTerraformCliArgs(argsToAppend ...string) {
	opts.TerraformCliArgs = append(opts.TerraformCliArgs, argsToAppend...)
}

// TerraformDataDir returns the Terraform data directory from TF_DATA_DIR env var or the default.
func (opts *RunnerOptions) TerraformDataDir() string {
	if tfDataDir, ok := opts.Env["TF_DATA_DIR"]; ok {
		return tfDataDir
	}
	return options.DefaultTFDataDir
}

// DataDir returns the Terraform data directory prepended with the working directory,
// or just the Terraform data directory if it is an absolute path.
func (opts *RunnerOptions) DataDir() string {
	tfDataDir := opts.TerraformDataDir()
	if filepath.IsAbs(tfDataDir) {
		return tfDataDir
	}
	return util.JoinPath(opts.WorkingDir, tfDataDir)
}

// Helper functions

// checkIfPlanFile checks if the argument is a terraform plan file.
func checkIfPlanFile(arg string) bool {
	return util.IsFile(arg) && filepath.Ext(arg) == ".tfplan"
}

// extractPlanFile extracts the plan file from the arguments list if present.
func extractPlanFile(argsToInsert []string) (*string, []string) {
	planFile := ""
	var filteredArgs []string

	for _, arg := range argsToInsert {
		if checkIfPlanFile(arg) {
			planFile = arg
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	if planFile != "" {
		return &planFile, filteredArgs
	}
	return nil, filteredArgs
}

// RunWithErrorHandling runs the given operation and handles any errors according to the configuration.
// This implements error retry and ignore logic based on the Errors configuration.
func (opts *RunnerOptions) RunWithErrorHandling(ctx context.Context, l log.Logger, r *report.Report, operation func() error) error {
	if opts.Errors == nil {
		return operation()
	}

	currentAttempt := 1

	// convert working dir to an absolute path for reporting
	absWorkingDir, err := filepath.Abs(opts.WorkingDir)
	if err != nil {
		return err
	}

	for {
		err := operation()
		if err == nil {
			return nil
		}

		// Process the error through our error handling configuration
		action, processErr := opts.Errors.ProcessError(l, err, currentAttempt)
		if processErr != nil {
			return fmt.Errorf("error processing error handling rules: %w", processErr)
		}

		if action == nil {
			return err
		}

		if action.ShouldIgnore {
			l.Warnf("Ignoring error, reason: %s", action.IgnoreMessage)

			// Handle ignore signals if any are configured
			if len(action.IgnoreSignals) > 0 {
				if err := opts.handleIgnoreSignals(l, action.IgnoreSignals); err != nil {
					return err
				}
			}

			run, err := r.EnsureRun(absWorkingDir)
			if err != nil {
				return err
			}

			if err := r.EndRun(
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
			// Respect --no-auto-retry flag
			if !opts.AutoRetry {
				return err
			}

			l.Warnf(
				"Encountered retryable error: %s\nAttempt %d of %d. Waiting %d second(s) before retrying...",
				action.RetryMessage,
				currentAttempt,
				action.RetryAttempts,
				action.RetrySleepSecs,
			)

			// Record that a retry will be attempted without prematurely marking success.
			run, err := r.EnsureRun(absWorkingDir)
			if err != nil {
				return err
			}

			if err := r.EndRun(
				run.Path,
				report.WithResult(report.ResultSucceeded),
				report.WithReason(report.ReasonRetrySucceeded),
				report.WithCauseRetryBlock(action.RetryBlockName),
			); err != nil {
				return err
			}

			// Sleep before retry
			select {
			case <-time.After(time.Duration(action.RetrySleepSecs) * time.Second):
				// try again
			case <-ctx.Done():
				return errors.New(ctx.Err())
			}

			currentAttempt++

			continue
		}

		return err
	}
}

// handleIgnoreSignals writes the ignore signals to the error-signals.json file.
func (opts *RunnerOptions) handleIgnoreSignals(l log.Logger, signals map[string]any) error {
	workingDir := opts.WorkingDir
	signalsFile := filepath.Join(workingDir, options.DefaultSignalsFile)

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
