// Package types provides runner-specific option structures and utilities.
// RunnerOptions is a thin wrapper around options.RuntimeOptions so downstream
// runner code doesn't need to reach for the full TerragruntOptions god object.
package types

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
)

// RunnerOptions contains only the fields the runner needs, sourced from options.RuntimeOptions.
type RunnerOptions struct {
	options.RuntimeOptions
}

// FromTerragruntOptions extracts RunnerOptions from TerragruntOptions.
func FromTerragruntOptions(opts *options.TerragruntOptions) *RunnerOptions {
	if opts == nil {
		return nil
	}

	return &RunnerOptions{
		RuntimeOptions: *opts.RuntimeOptions.Clone(),
	}
}

// ToTerragruntOptions converts RunnerOptions back to TerragruntOptions.
// Useful for bridging to code that still expects the broader options structure.
func (opts *RunnerOptions) ToTerragruntOptions() *options.TerragruntOptions {
	if opts == nil {
		return nil
	}

	return &options.TerragruntOptions{
		RuntimeOptions: *opts.RuntimeOptions.Clone(),
	}
}

// Clone creates a deep copy of RunnerOptions.
func (opts *RunnerOptions) Clone() *RunnerOptions {
	if opts == nil {
		return nil
	}

	return &RunnerOptions{
		RuntimeOptions: *opts.RuntimeOptions.Clone(),
	}
}

// CloneWithConfigPath creates a copy of RunnerOptions with a new config path and working directory.
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

	newOpts.TerragruntConfigPath = configPath
	newOpts.WorkingDir = filepath.Dir(configPath)

	return newOpts, nil
}

// InsertTerraformCliArgs inserts args after the terraform command but before remaining args.
func (opts *RunnerOptions) InsertTerraformCliArgs(argsToInsert ...string) {
	planFile, restArgs := extractPlanFile(argsToInsert)

	const (
		singleCommandLength = 1
		subCommandLength    = 2
	)

	commandLength := singleCommandLength
	if len(opts.TerraformCliArgs) > 0 && util.ListContainsElement(options.TerraformCommandsWithSubcommand, opts.TerraformCliArgs[0]) {
		commandLength = util.Min(subCommandLength, len(opts.TerraformCliArgs))
	}

	var args []string

	args = append(args, opts.TerraformCliArgs[:commandLength]...)
	args = append(args, restArgs...)
	args = append(args, opts.TerraformCliArgs[commandLength:]...)

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
func (opts *RunnerOptions) RunWithErrorHandling(ctx context.Context, l log.Logger, r *report.Report, operation func() error) error {
	if opts.Errors == nil {
		return operation()
	}

	currentAttempt := 1

	absWorkingDir, err := filepath.Abs(opts.WorkingDir)
	if err != nil {
		return err
	}

	for {
		err := operation()
		if err == nil {
			return nil
		}

		action, processErr := opts.Errors.ProcessError(l, err, currentAttempt)
		if processErr != nil {
			return fmt.Errorf("error processing error handling rules: %w", processErr)
		}

		if action == nil {
			return err
		}

		if action.ShouldIgnore {
			l.Warnf("Ignoring error, reason: %s", action.IgnoreMessage)

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
