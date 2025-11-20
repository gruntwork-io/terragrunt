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

func (runner *UnitRunner) runTerragrunt(ctx context.Context, opts *options.TerragruntOptions, r *report.Report) error {
	logger := runner.Unit.Logger()
	logger.Debugf("UnitRunner.runTerragrunt called for %s, opts.RunTerragrunt=%v", runner.Unit.Path(), opts.RunTerragrunt)
	logger.Debugf("Running %s", runner.Unit.Path())

	opts.Writer = component.NewUnitWriter(opts.Writer)

	defer func() {
		outputLocks.Lock(runner.Unit.Path())
		defer outputLocks.Unlock(runner.Unit.Path())

		runner.Unit.FlushOutput() //nolint:errcheck
	}()

	// Only create report entries if report is not nil
	if r != nil {
		// Component paths are always absolute from ingestion (via util.CanonicalPath)
		// CanonicalPath already cleans the path, so no additional cleaning needed
		run, err := report.NewRun(runner.Unit.Path())
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

	runErr := opts.RunTerragrunt(ctx, logger, opts, r)

	// Only merge the final unit exit code when the unit run completed without error
	// and the exit code isn't stuck at 1 from a prior retry attempt.
	if runErr == nil && globalExitCode != nil && unitExitCode.Get() != tf.DetailedExitCodeError {
		globalExitCode.Set(unitExitCode.Get())
	}

	// End the run with appropriate result (only if report is not nil)
	if r != nil {
		// Get the unit path (already computed above)
		unitPath := runner.Unit.AbsolutePath()
		unitPath = util.CleanPath(unitPath)
		logger := runner.Unit.Logger()

		if runErr != nil {
			if endErr := r.EndRun(
				unitPath,
				report.WithResult(report.ResultFailed),
				report.WithReason(report.ReasonRunError),
				report.WithCauseRunError(runErr.Error()),
			); endErr != nil {
				logger.Errorf("Error ending run for unit %s: %v", unitPath, endErr)
			}
		} else {
			if endErr := r.EndRun(unitPath, report.WithResult(report.ResultSucceeded)); endErr != nil {
				logger.Errorf("Error ending run for unit %s: %v", unitPath, endErr)
			}
		}
	}

	return runErr
}

// Run a unit right now by executing the runTerragrunt command with the provided options.
func (runner *UnitRunner) Run(ctx context.Context, opts *options.TerragruntOptions, r *report.Report) error {
	runner.Status = Running

	logger := runner.Unit.Logger()
	logger.Infof("[VERSION-DEBUG] UnitRunner.Run called for %s, working dir: %s", runner.Unit.Path(), opts.WorkingDir)

	if runner.Unit.AssumeAlreadyApplied() {
		logger.Debugf("Assuming unit %s has already been applied and skipping it", runner.Unit.Path())
		return nil
	}

	if err := runner.runTerragrunt(ctx, opts, r); err != nil {
		return err
	}

	// convert terragrunt output to json
	if runner.Unit.GetOutputJSONFile() != "" {
		l, jsonOptions, err := opts.CloneWithConfigPath(logger, opts.TerragruntConfigPath)
		if err != nil {
			return err
		}

		stdout := bytes.Buffer{}
		jsonOptions.ForwardTFStdout = true
		jsonOptions.JSONLogFormat = false
		jsonOptions.Writer = &stdout
		jsonOptions.TerraformCommand = tf.CommandNameShow
		jsonOptions.TerraformCliArgs = []string{tf.CommandNameShow, "-json", runner.Unit.PlanFile()}

		// Use an ad-hoc report to avoid polluting the main report with entries
		// for the cache directory, while still satisfying RunTerragrunt's
		// expectation for a non-nil report parameter.
		adhocReport := report.NewReport()
		if err := jsonOptions.RunTerragrunt(ctx, l, jsonOptions, adhocReport); err != nil {
			return err
		}

		// save the json output to the file plan file
		// Note: Unit should already have ExecutionOptions set from Run() method
		outputFile := runner.Unit.GetOutputJSONFile()
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
