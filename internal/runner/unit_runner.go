package runner

import (
	"context"
	"io"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
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

// UnitRunner runs a single [component.Unit].
type UnitRunner struct {
	Err    error
	Unit   *component.Unit
	Status UnitStatus
}

// NewUnitRunner returns a [UnitRunner] for unit.
func NewUnitRunner(unit *component.Unit) *UnitRunner {
	return &UnitRunner{
		Unit:   unit,
		Status: Waiting,
	}
}

func (runner *UnitRunner) runTerragrunt(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	r *report.Report,
	cfg *runcfg.RunConfig,
	credsGetter *creds.Getter,
) error {
	l.Debugf(
		"Running %s",
		util.RelPathForLog(opts.RootWorkingDir, runner.Unit.Path(), opts.LogShowAbsPaths),
	)

	defer func() {
		// Flush buffered output for this unit, if the writer supports it.
		if err := component.FlushOutput(runner.Unit, v.Writers.Writer); err != nil {
			l.Errorf("Error flushing output for unit %s: %v", runner.Unit.Path(), err)
		}
	}()

	if r != nil {
		unitPath := runner.Unit.Path()
		unitPath = filepath.Clean(unitPath)

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

	runErr := run.Run(ctx, l, v, configbridge.NewRunOptions(opts), r, cfg, credsGetter)

	if globalExitCode != nil {
		unitPath := runner.Unit.Path()
		code := unitExitCode.Get(unitPath)
		globalExitCode.Set(unitPath, code)
	}

	if r != nil {
		unitPath := runner.Unit.Path()
		unitPath = filepath.Clean(unitPath)

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

// Run executes the unit and, when the unit has a JSON output file configured,
// writes its plan out as JSON.
func (runner *UnitRunner) Run(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	r *report.Report,
	cfg *runcfg.RunConfig,
	credsGetter *creds.Getter,
) error {
	runner.Status = Running

	if opts == nil {
		return nil
	}

	if err := runner.runTerragrunt(ctx, l, v, opts, r, cfg, credsGetter); err != nil {
		return err
	}

	if runner.Unit.OutputJSONFile(opts.RootWorkingDir, opts.JSONOutputFolder) != "" {
		jsonLogger, jsonOptions, err := opts.CloneWithConfigPath(
			l,
			opts.TerragruntConfigPath,
		)
		if err != nil {
			return err
		}

		jsonOptions.ForwardTFStdout = true
		jsonOptions.JSONLogFormat = false
		jsonOptions.TerraformCommand = tf.CommandNameShow
		planFile := runner.Unit.PlanFile(
			opts.RootWorkingDir, opts.OutputFolder, opts.JSONOutputFolder, opts.TerraformCommand,
		)
		jsonOptions.TerraformCliArgs = iacargs.New(tf.CommandNameShow, "-json", planFile)

		// Use an ad-hoc report to avoid polluting the main report
		adhocReport := report.NewReport()

		runOpts := configbridge.NewRunOptions(jsonOptions)
		outputFile := runner.Unit.OutputJSONFile(opts.RootWorkingDir, opts.JSONOutputFolder)

		if err := WriteJSONOutput(v.FS, outputFile, func(w io.Writer) error {
			return run.Run(
				ctx,
				jsonLogger,
				v.WithWriter(w),
				runOpts,
				adhocReport,
				cfg,
				credsGetter,
			)
		}); err != nil {
			return err
		}
	}

	return nil
}
