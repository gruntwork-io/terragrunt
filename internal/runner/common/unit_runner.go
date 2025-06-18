package common

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
)

// ModuleStatus represents the status of a module that we are
// trying to apply or destroy as part of the run --all apply or run --all destroy command
type ModuleStatus int

const (
	Waiting ModuleStatus = iota
	Running
	Finished
)

// UnitRunner handles the logic for running a single module.
type UnitRunner struct {
	Err          error
	Module       *Unit
	Logger       log.Logger
	Status       ModuleStatus
	FlagExcluded bool
}

var outputMu sync.Mutex

func NewUnitRunner(module *Unit) *UnitRunner {
	return &UnitRunner{
		Module:       module,
		Status:       Waiting,
		Logger:       module.Logger,
		FlagExcluded: module.FlagExcluded,
	}
}

func (module *UnitRunner) RunTerragrunt(ctx context.Context, opts *options.TerragruntOptions, r *report.Report) error {
	module.Logger.Debugf("Running %s", module.Module.Path)

	opts.Writer = NewModuleWriter(opts.Writer)

	defer func() {
		outputMu.Lock()
		defer outputMu.Unlock()
		module.Module.FlushOutput() //nolint:errcheck
	}()

	if opts.Experiments.Evaluate(experiment.Report) {
		run, err := report.NewRun(module.Module.Path)
		if err != nil {
			return err
		}

		if err := r.AddRun(run); err != nil {
			return err
		}
	}

	return opts.RunTerragrunt(ctx, module.Logger, opts, r)
}

// Run a module right now by executing the RunTerragrunt command of its TerragruntOptions field.
func (runner *UnitRunner) RunNow(ctx context.Context, rootOptions *options.TerragruntOptions, r *report.Report) error {
	runner.Status = Running

	if runner.Module.AssumeAlreadyApplied {
		runner.Logger.Debugf("Assuming module %s has already been applied and skipping it", runner.Module.Path)
		return nil
	} else {
		if err := runner.RunTerragrunt(ctx, runner.Module.TerragruntOptions, r); err != nil {
			return err
		}

		// convert terragrunt output to json
		if runner.Module.OutputJSONFile(runner.Logger, runner.Module.TerragruntOptions) != "" {
			l, jsonOptions, err := runner.Module.TerragruntOptions.CloneWithConfigPath(runner.Logger, runner.Module.TerragruntOptions.TerragruntConfigPath)
			if err != nil {
				return err
			}

			stdout := bytes.Buffer{}
			jsonOptions.ForwardTFStdout = true
			jsonOptions.JSONLogFormat = false
			jsonOptions.Writer = &stdout
			jsonOptions.TerraformCommand = tf.CommandNameShow
			jsonOptions.TerraformCliArgs = []string{tf.CommandNameShow, "-json", runner.Module.PlanFile(l, rootOptions)}

			if err := jsonOptions.RunTerragrunt(ctx, l, jsonOptions, r); err != nil {
				return err
			}

			// save the json output to the file plan file
			outputFile := runner.Module.OutputJSONFile(l, rootOptions)
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
}
