package common

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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

// UnitRunner handles the logic for running a single unit.
type UnitRunner struct {
	Err    error
	Unit   *component.Unit
	Status UnitStatus
}

var outputLocks = util.NewKeyLocks()

func NewUnitRunner(unit *component.Unit) *UnitRunner {
	return &UnitRunner{
		Unit:   unit,
		Status: Waiting,
	}
}

func (runner *UnitRunner) runTerragrunt(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, r *report.Report) error {
	l.Debugf("Running %s", runner.Unit.Path())

	opts.Writer = component.NewUnitWriter(opts.Writer)

	defer func() {
		outputLocks.Lock(runner.Unit.Path())
		defer outputLocks.Unlock(runner.Unit.Path())

		runner.Unit.FlushOutput(l) //nolint:errcheck
	}()

	// Only create report entries if report is not nil
	if r != nil {
		// Ensure path is absolute and normalized for reporting
		unitPath, err := component.EnsureAbsolutePath(runner.Unit.Path())
		if err != nil {
			return err
		}

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

	runErr := opts.RunTerragrunt(ctx, l, opts, r)

	// Only merge the final unit exit code when the unit run completed without error
	// and the exit code isn't stuck at 1 from a prior retry attempt.
	if runErr == nil && globalExitCode != nil && unitExitCode.Get() != tf.DetailedExitCodeError {
		globalExitCode.Set(unitExitCode.Get())
	}

	// End the run with appropriate result (only if report is not nil)
	if r != nil {
		// Get the unit path (already computed above)
		unitPath := runner.Unit.AbsolutePath(l)
		unitPath = util.CleanPath(unitPath)

		if runErr != nil {
			if endErr := r.EndRun(
				unitPath,
				report.WithResult(report.ResultFailed),
				report.WithReason(report.ReasonRunError),
				report.WithCauseRunError(runErr.Error()),
			); endErr != nil {
				l.Errorf("Error ending run for unit %s: %v", unitPath, endErr)
			}
		} else {
			if endErr := r.EndRun(unitPath, report.WithResult(report.ResultSucceeded)); endErr != nil {
				l.Errorf("Error ending run for unit %s: %v", unitPath, endErr)
			}
		}
	}

	return runErr
}

// Run a unit right now by executing the runTerragrunt command of its TerragruntOptions field.
func (runner *UnitRunner) Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, r *report.Report) error {
	runner.Status = Running

	if runner.Unit.External() && !runner.Unit.ShouldApplyExternal() {
		l.Debugf("Assuming unit %s has already been applied and skipping it", runner.Unit.Path())
		return nil
	}

	if err := runner.runTerragrunt(ctx, l, runner.Unit.Opts(), r); err != nil {
		return err
	}

	// convert terragrunt output to json
	if runner.Unit.OutputJSONFile(l, runner.Unit.Opts()) != "" {
		l, jsonOptions, err := runner.Unit.Opts().CloneWithConfigPath(l, runner.Unit.Opts().TerragruntConfigPath)
		if err != nil {
			return err
		}

		stdout := bytes.Buffer{}
		jsonOptions.ForwardTFStdout = true
		jsonOptions.JSONLogFormat = false
		jsonOptions.Writer = &stdout
		jsonOptions.TerraformCommand = tf.CommandNameShow
		jsonOptions.TerraformCliArgs = []string{tf.CommandNameShow, "-json", runner.Unit.PlanFile(l, opts)}

		// Use an ad-hoc report to avoid polluting the main report with entries
		// for the cache directory, while still satisfying RunTerragrunt's
		// expectation for a non-nil report parameter.
		adhocReport := report.NewReport()
		if err := jsonOptions.RunTerragrunt(ctx, l, jsonOptions, adhocReport); err != nil {
			return err
		}

		// save the json output to the file plan file
		outputFile := runner.Unit.OutputJSONFile(l, opts)
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
