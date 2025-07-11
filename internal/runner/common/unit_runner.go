package common

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
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
	Unit   *Unit
	Status UnitStatus
}

var outputLocks = util.NewKeyLocks()

func NewUnitRunner(unit *Unit) *UnitRunner {
	return &UnitRunner{
		Unit:   unit,
		Status: Waiting,
	}
}

func (runner *UnitRunner) runTerragrunt(ctx context.Context, opts *options.TerragruntOptions, r *report.Report) error {
	runner.Unit.Logger.Debugf("Running %s", runner.Unit.Path)

	opts.Writer = NewUnitWriter(opts.Writer)

	defer func() {
		outputLocks.Lock(runner.Unit.Path)
		defer outputLocks.Unlock(runner.Unit.Path)
		runner.Unit.FlushOutput() //nolint:errcheck
	}()

	if opts.Experiments.Evaluate(experiment.Report) {
		run, err := report.NewRun(runner.Unit.Path)
		if err != nil {
			return err
		}

		if err := r.AddRun(run); err != nil {
			return err
		}
	}

	return opts.RunTerragrunt(ctx, runner.Unit.Logger, opts, r)
}

// Run a unit right now by executing the runTerragrunt command of its TerragruntOptions field.
func (runner *UnitRunner) Run(ctx context.Context, opts *options.TerragruntOptions, r *report.Report) error {
	runner.Status = Running

	if runner.Unit.AssumeAlreadyApplied {
		runner.Unit.Logger.Debugf("Assuming unit %s has already been applied and skipping it", runner.Unit.Path)
		return nil
	}

	if err := runner.runTerragrunt(ctx, runner.Unit.TerragruntOptions, r); err != nil {
		return err
	}

	// convert terragrunt output to json
	if runner.Unit.OutputJSONFile(runner.Unit.Logger, runner.Unit.TerragruntOptions) != "" {
		l, jsonOptions, err := runner.Unit.TerragruntOptions.CloneWithConfigPath(runner.Unit.Logger, runner.Unit.TerragruntOptions.TerragruntConfigPath)
		if err != nil {
			return err
		}

		stdout := bytes.Buffer{}
		jsonOptions.ForwardTFStdout = true
		jsonOptions.JSONLogFormat = false
		jsonOptions.Writer = &stdout
		jsonOptions.TerraformCommand = tf.CommandNameShow
		jsonOptions.TerraformCliArgs = []string{tf.CommandNameShow, "-json", runner.Unit.PlanFile(l, opts)}

		if err := jsonOptions.RunTerragrunt(ctx, l, jsonOptions, r); err != nil {
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
