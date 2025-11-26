package common

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

// UnitStatus represents the status of a unit that we are
// trying to apply or destroy as part of the run --all apply or run --all destroy command
type UnitStatus int

const (
	Waiting UnitStatus = iota
	Running
	Finished
)

// (legacy common.Unit runner removed; use ComponentUnitRunner below)

// ComponentUnitRunner handles the logic for running a single component.Unit.
type ComponentUnitRunner struct {
	Err    error
	Unit   *component.Unit
	Status UnitStatus
}

// NewUnitRunnerFromComponent creates a UnitRunner from a component.Unit.
func NewUnitRunnerFromComponent(unit *component.Unit) *ComponentUnitRunner {
	return &ComponentUnitRunner{
		Unit:   unit,
		Status: Waiting,
	}
}

func (runner *ComponentUnitRunner) runTerragrunt(ctx context.Context, opts *options.TerragruntOptions, r *report.Report) error {
	if runner.Unit.Execution == nil || runner.Unit.Execution.Logger == nil {
		return nil
	}

	runner.Unit.Execution.Logger.Debugf("Running %s", runner.Unit.Path())

	opts.Writer = NewUnitWriter(opts.Writer)

	defer func() {
		// Flush buffered output for this unit, if the writer supports it.
		_ = component.FlushOutput(runner.Unit)
	}()

	// Only create report entries if report is not nil
	if r != nil {
		unitPath := runner.Unit.AbsolutePath()
		unitPath = util.CleanPath(unitPath)

		run, err := report.NewRun(unitPath)
		if err != nil {
			return err
		}

		if err := r.AddRun(run); err != nil {
			return err
		}
	}

	// Use a unit-scoped detailed exit code so retries in this unit don't clobber global state
	globalExitCode := tf.DetailedExitCodeFromContext(ctx)

	var unitExitCode tf.DetailedExitCode

	ctx = tf.ContextWithDetailedExitCode(ctx, &unitExitCode)

	runErr := opts.RunTerragrunt(ctx, runner.Unit.Execution.Logger, opts, r)

	// Only merge the final unit exit code when the unit run completed without error
	// and the exit code isn't stuck at 1 from a prior retry attempt.
	if runErr == nil && globalExitCode != nil && unitExitCode.Get() != tf.DetailedExitCodeError {
		globalExitCode.Set(unitExitCode.Get())
	}

	// End the run with appropriate result (only if report is not nil)
	if r != nil {
		unitPath := runner.Unit.AbsolutePath()
		unitPath = util.CleanPath(unitPath)

		if runErr != nil {
			if endErr := r.EndRun(
				unitPath,
				report.WithResult(report.ResultFailed),
				report.WithReason(report.ReasonRunError),
				report.WithCauseRunError(runErr.Error()),
			); endErr != nil {
				runner.Unit.Execution.Logger.Errorf("Error ending run for unit %s: %v", unitPath, endErr)
			}
		} else {
			if endErr := r.EndRun(unitPath, report.WithResult(report.ResultSucceeded)); endErr != nil {
				runner.Unit.Execution.Logger.Errorf("Error ending run for unit %s: %v", unitPath, endErr)
			}
		}
	}

	return runErr
}

// Run executes a component.Unit right now.
func (runner *ComponentUnitRunner) Run(ctx context.Context, opts *options.TerragruntOptions, r *report.Report) error {
	runner.Status = Running

	if runner.Unit.Execution == nil {
		return nil
	}

	if runner.Unit.Execution.AssumeAlreadyApplied {
		if runner.Unit.Execution.Logger != nil {
			runner.Unit.Execution.Logger.Debugf("Assuming unit %s has already been applied and skipping it", runner.Unit.Path())
		}

		return nil
	}

	if err := runner.runTerragrunt(ctx, runner.Unit.Execution.TerragruntOptions, r); err != nil {
		return err
	}

	// convert terragrunt output to json
	if runner.Unit.OutputJSONFile(runner.Unit.Execution.TerragruntOptions) != "" {
		l, jsonOptions, err := runner.Unit.Execution.TerragruntOptions.CloneWithConfigPath(runner.Unit.Execution.Logger, runner.Unit.Execution.TerragruntOptions.TerragruntConfigPath)
		if err != nil {
			return err
		}

		stdout := bytes.Buffer{}
		jsonOptions.ForwardTFStdout = true
		jsonOptions.JSONLogFormat = false
		jsonOptions.Writer = &stdout
		jsonOptions.TerraformCommand = tf.CommandNameShow
		jsonOptions.TerraformCliArgs = []string{tf.CommandNameShow, "-json", runner.Unit.PlanFile(opts)}

		// Use an ad-hoc report to avoid polluting the main report
		adhocReport := report.NewReport()
		if err := jsonOptions.RunTerragrunt(ctx, l, jsonOptions, adhocReport); err != nil {
			return err
		}

		// save the json output to the file plan file
		outputFile := runner.Unit.OutputJSONFile(opts)
		jsonDir := filepath.Dir(outputFile)

		if err := os.MkdirAll(jsonDir, os.ModePerm); err != nil {
			return err
		}

		if err := os.WriteFile(outputFile, stdout.Bytes(), os.ModePerm); err != nil {
			return err
		}
	}

	return nil
}
