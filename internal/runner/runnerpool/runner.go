// Package runnerpool provides a runner implementation based on a pool pattern for executing multiple units concurrently.
package runnerpool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"

	"github.com/gruntwork-io/terragrunt/internal/errors"

	"github.com/gruntwork-io/terragrunt/tf"

	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Runner implements the Stack interface for runner pool execution.
type Runner struct {
	Stack *runbase.Stack
}

// NewRunnerPoolStack creates a new stack from discovered units.
func NewRunnerPoolStack(l log.Logger, terragruntOptions *options.TerragruntOptions, discovered discovery.DiscoveredConfigs, opts ...runbase.Option) (runbase.StackRunner, error) {
	unitsMap := make(runbase.UnitsMap, len(discovered))

	stack := runbase.Stack{
		TerragruntOptions: terragruntOptions,
	}

	runner := &Runner{
		Stack: &stack,
	}

	terragruntConfigPaths := make([]string, 0, len(discovered))

	for _, cfg := range discovered {
		configPath := config.GetDefaultConfigPath(cfg.Path)

		if cfg.Parsed == nil {
			// Skip configurations that could not be parsed
			l.Warnf("Skipping unit at %s due to parse error", cfg.Path)
			continue
		}

		modLogger, modOpts, err := terragruntOptions.CloneWithConfigPath(l, configPath)

		if err != nil {
			l.Warnf("Skipping unit at %s due to error cloning options: %s", cfg.Path, err)
			continue // skip on error
		}

		mod := &runbase.Unit{
			TerragruntOptions: modOpts,
			Logger:            modLogger,
			Path:              cfg.Path,
			Config:            *cfg.Parsed,
		}

		terragruntConfigPaths = append(terragruntConfigPaths, configPath)

		unitsMap[cfg.Path] = mod
	}

	canonicalTerragruntConfigPaths, err := util.CanonicalPaths(terragruntConfigPaths, ".")
	if err != nil {
		return nil, err
	}

	linkedUnits, err := unitsMap.CrossLinkDependencies(canonicalTerragruntConfigPaths)
	if err != nil {
		return nil, err
	}
	// Reorder linkedUnits to match the order of canonicalTerragruntConfigPaths
	orderedUnits := make(runbase.Units, 0, len(canonicalTerragruntConfigPaths))
	pathToUnit := make(map[string]*runbase.Unit)

	for _, m := range linkedUnits {
		pathToUnit[config.GetDefaultConfigPath(m.Path)] = m
	}

	for _, configPath := range canonicalTerragruntConfigPaths {
		if m, ok := pathToUnit[configPath]; ok {
			orderedUnits = append(orderedUnits, m)
		} else {
			l.Warnf("Unit for config path %s not found in linked units", configPath)
		}
	}

	stack.Units = orderedUnits

	return runner.WithOptions(opts...), nil
}

func (r *Runner) Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	stackCmd := opts.TerraformCommand

	if opts.OutputFolder != "" {
		for _, u := range r.Stack.Units {
			planFile := u.OutputFile(l, opts)
			if err := os.MkdirAll(filepath.Dir(planFile), os.ModePerm); err != nil {
				return err
			}
		}
	}
	if util.ListContainsElement(config.TerraformCommandsNeedInput, stackCmd) {
		opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-input=false", 1)
		r.syncTerraformCliArgs(l, opts)
	}
	switch stackCmd {
	case tf.CommandNameApply, tf.CommandNameDestroy:
		if opts.RunAllAutoApprove {
			opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-auto-approve", 1)
		}
		r.syncTerraformCliArgs(l, opts)
	case tf.CommandNameShow:
		r.syncTerraformCliArgs(l, opts)
	case tf.CommandNamePlan:
		errStreams := make([]bytes.Buffer, len(r.Stack.Units))
		for i, u := range r.Stack.Units {
			u.TerragruntOptions.ErrWriter = io.MultiWriter(&errStreams[i], u.TerragruntOptions.ErrWriter)
		}
		defer r.summarizePlanAllErrors(l, errStreams)
	}

	//------------------------------------------------------------------
	// 1. Build the per-task runner function
	//------------------------------------------------------------------
	runTask := func(ctx context.Context, t *Task) Result {
		unitRunner := runbase.NewUnitRunner(t.Unit)

		err := unitRunner.Run(ctx, t.Unit.TerragruntOptions, r.Stack.Report)

		res := Result{TaskID: t.ID()}
		if err != nil {
			res.Err = err
			res.ExitCode = 1
		}
		return res
	}

	maxConc := opts.Parallelism
	pool := New(r.Stack.Units, runTask, maxConc, false)

	results := pool.Run(ctx)

	var errs []error
	for _, r := range results {
		if r.Err != nil {
			errs = append(errs, r.Err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (runner *Runner) LogUnitDeployOrder(l log.Logger, terraformCommand string) error {
	outStr := fmt.Sprintf("The runner-pool runner at %s will be processed in the following order for command %s:\n", runner.Stack.TerragruntOptions.WorkingDir, terraformCommand)
	for _, unit := range runner.Stack.Units {
		outStr += fmt.Sprintf("Unit %s\n", unit.Path)
	}

	l.Info(outStr)

	return nil
}

func (runner *Runner) JSONUnitDeployOrder(terraformCommand string) (string, error) {
	orderedUnits := make([]string, 0, len(runner.Stack.Units))
	for _, unit := range runner.Stack.Units {
		orderedUnits = append(orderedUnits, unit.Path)
	}

	j, err := json.MarshalIndent(orderedUnits, "", "  ")
	if err != nil {
		return "", err
	}

	return string(j), nil
}

func (runner *Runner) ListStackDependentUnits() map[string][]string {
	dependentUnits := make(map[string][]string)

	for _, unit := range runner.Stack.Units {
		if len(unit.Dependencies) != 0 {
			for _, dep := range unit.Dependencies {
				dependentUnits[dep.Path] = util.RemoveDuplicatesFromList(append(dependentUnits[dep.Path], unit.Path))
			}
		}
	}

	for {
		noUpdates := true

		for unit, dependents := range dependentUnits {
			for _, dependent := range dependents {
				initialSize := len(dependentUnits[unit])
				list := util.RemoveDuplicatesFromList(append(dependentUnits[unit], dependentUnits[dependent]...))
				list = util.RemoveElementFromList(list, unit)
				dependentUnits[unit] = list

				if initialSize != len(dependentUnits[unit]) {
					noUpdates = false
				}
			}
		}

		if noUpdates {
			break
		}
	}

	return dependentUnits
}

// Sync the TerraformCliArgs for each unit in the stack to match the provided terragruntOptions struct.
func (runner *Runner) syncTerraformCliArgs(l log.Logger, opts *options.TerragruntOptions) {
	for _, unit := range runner.Stack.Units {
		unit.TerragruntOptions.TerraformCliArgs = make([]string, len(opts.TerraformCliArgs))
		copy(unit.TerragruntOptions.TerraformCliArgs, opts.TerraformCliArgs)

		planFile := unit.PlanFile(l, opts)
		if planFile != "" {
			l.Debugf("Using output file %s for unit %s", planFile, unit.TerragruntOptions.TerragruntConfigPath)

			if unit.TerragruntOptions.TerraformCommand == "plan" {
				// for plan command add -out=<file> to the terraform cli args
				unit.TerragruntOptions.TerraformCliArgs = append(unit.TerragruntOptions.TerraformCliArgs, "-out="+planFile)
			} else {
				unit.TerragruntOptions.TerraformCliArgs = append(unit.TerragruntOptions.TerraformCliArgs, planFile)
			}
		}
	}
}

// We inspect the error streams to give an explicit message if the plan failed because there were references to
// remote states. `terraform plan` will fail if it tries to access remote state from dependencies and the plan
// has never been applied on the dependency.
func (runner *Runner) summarizePlanAllErrors(l log.Logger, errorStreams []bytes.Buffer) {
	for i, errorStream := range errorStreams {
		output := errorStream.String()

		if len(output) == 0 {
			// We get empty buffer if runner execution completed without errors, so skip that to avoid logging too much
			continue
		}

		if strings.Contains(output, "Error running plan:") && strings.Contains(output, ": Resource 'data.terraform_remote_state.") {
			var dependenciesMsg string
			if len(runner.Stack.Units[i].Dependencies) > 0 {
				dependenciesMsg = fmt.Sprintf(" contains dependencies to %v and", runner.Stack.Units[i].Config.Dependencies.Paths)
			}

			l.Infof("%v%v refers to remote state "+
				"you may have to apply your changes in the dependencies prior running terragrunt run --all plan.\n",
				runner.Stack.Units[i].Path,
				dependenciesMsg,
			)
		}
	}
}

// WithOptions updates the stack with the provided options.
func (runner *Runner) WithOptions(opts ...runbase.Option) *Runner {
	for _, opt := range opts {
		opt(runner)
	}

	return runner
}

func (runner *Runner) GetStack() *runbase.Stack {
	return runner.Stack
}

// SetTerragruntConfig sets the report for the stack.
func (runner *Runner) SetTerragruntConfig(config *config.TerragruntConfig) {
	runner.Stack.ChildTerragruntConfig = config
}

// SetParseOptions sets the report for the stack.
func (runner *Runner) SetParseOptions(parserOptions []hclparse.Option) {
	runner.Stack.ParserOptions = parserOptions
}

// SetReport sets the report for the stack.
func (runner *Runner) SetReport(report *report.Report) {
	runner.Stack.Report = report
}
