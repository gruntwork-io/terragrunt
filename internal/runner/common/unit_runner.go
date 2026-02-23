package common

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// UnitStatus represents the status of a unit during execution.
type UnitStatus int

const (
	Waiting UnitStatus = iota
	Running
	Finished
)

// UnitRunner handles the logic for running a single component.Unit.
type UnitRunner struct {
	Err    error
	Unit   *component.Unit
	Status UnitStatus
}

// NewUnitRunner creates a UnitRunner from a component.Unit.
func NewUnitRunner(unit *component.Unit) *UnitRunner {
	return &UnitRunner{
		Unit:   unit,
		Status: Waiting,
	}
}

func (runner *UnitRunner) runTerragrunt(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	r *report.Report,
	cfg *runcfg.RunConfig,
	credsGetter *creds.Getter,
) error {
	l.Debugf("Running %s", util.RelPathForLog(opts.RootWorkingDir, runner.Unit.Path(), opts.LogShowAbsPaths))

	defer func() {
		// Flush buffered output for this unit, if the writer supports it.
		if err := component.FlushOutput(runner.Unit, opts.Writer); err != nil {
			l.Errorf("Error flushing output for unit %s: %v", runner.Unit.Path(), err)
		}
	}()

	// Only create report entries if report is not nil
	if r != nil {
		unitPath := runner.Unit.Path()
		unitPath = util.CleanPath(unitPath)

		// Pass the discovery context fields for worktree scenarios
		var ensureOpts []report.EndOption

		if discoveryCtx := runner.Unit.DiscoveryContext(); discoveryCtx != nil {
			ensureOpts = append(
				ensureOpts,
				report.WithDiscoveryWorkingDir(discoveryCtx.WorkingDir),
				report.WithRef(discoveryCtx.Ref),
				report.WithCmd(discoveryCtx.Cmd),
				report.WithArgs(discoveryCtx.Args),
			)
		}

		if _, err := r.EnsureRun(l, unitPath, ensureOpts...); err != nil {
			return err
		}
	}

	// Use a unit-scoped detailed exit code so retries in this unit don't clobber global state
	globalExitCode := tf.DetailedExitCodeFromContext(ctx)

	unitExitCode := tf.NewDetailedExitCodeMap()

	ctx = tf.ContextWithDetailedExitCode(ctx, unitExitCode)

	runErr := run.Run(ctx, l, opts, r, cfg, credsGetter)

	// Store the unit exit code in the global map using the unit path as key.
	if globalExitCode != nil {
		unitPath := runner.Unit.Path()
		code := unitExitCode.Get(unitPath)
		globalExitCode.Set(unitPath, code)
	}

	// End the run with appropriate result (only if report is not nil)
	if r != nil {
		unitPath := runner.Unit.Path()
		unitPath = util.CleanPath(unitPath)

		if runErr != nil {
			if endErr := r.EndRun(
				l,
				unitPath,
				report.WithResult(report.ResultFailed),
				report.WithReason(report.ReasonRunError),
				report.WithCauseRunError(runErr.Error()),
			); endErr != nil {
				l.Errorf("Error ending run for unit %s: %v", unitPath, endErr)
			}
		} else {
			if endErr := r.EndRun(
				l,
				unitPath,
				report.WithResult(report.ResultSucceeded),
			); endErr != nil {
				l.Errorf("Error ending run for unit %s: %v", unitPath, endErr)
			}
		}
	}

	return runErr
}

// Run executes a component.Unit right now.
func (runner *UnitRunner) Run(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	r *report.Report,
	cfg *runcfg.RunConfig,
	credsGetter *creds.Getter,
) error {
	runner.Status = Running

	if opts == nil {
		return nil
	}

	if err := runner.runTerragrunt(ctx, l, opts, r, cfg, credsGetter); err != nil {
		return err
	}

	// convert terragrunt output to json
	if runner.Unit.OutputJSONFile(opts.RootWorkingDir, opts.JSONOutputFolder) != "" {
		jsonLogger, jsonOptions, err := opts.CloneWithConfigPath(
			l,
			opts.TerragruntConfigPath,
		)
		if err != nil {
			return err
		}

		stdout := bytes.Buffer{}
		jsonOptions.ForwardTFStdout = true
		jsonOptions.JSONLogFormat = false
		jsonOptions.Writer = &stdout
		jsonOptions.TerraformCommand = tf.CommandNameShow
		jsonOptions.TerraformCliArgs = iacargs.New(tf.CommandNameShow, "-json", runner.Unit.PlanFile(opts.RootWorkingDir, opts.OutputFolder, opts.JSONOutputFolder, opts.TerraformCommand))

		// Use an ad-hoc report to avoid polluting the main report
		adhocReport := report.NewReport()
		if err := run.Run(ctx, jsonLogger, jsonOptions, adhocReport, cfg, credsGetter); err != nil {
			return err
		}

		// save the json output to the file plan file
		outputFile := runner.Unit.OutputJSONFile(opts.RootWorkingDir, opts.JSONOutputFolder)
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
