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
	Stack            *runbase.Stack
	planErrorBuffers []bytes.Buffer // Store plan error buffers for summarizePlanAllErrors
}

// NewRunnerPoolStack creates a new stack from discovered units.
func NewRunnerPoolStack(l log.Logger, terragruntOptions *options.TerragruntOptions, discovered discovery.DiscoveredConfigs, opts ...runbase.Option) (runbase.StackRunner, error) {
	unitsMap := make(runbase.UnitsMap, len(discovered))
	orderedUnits := make(runbase.Units, 0, len(discovered))

	stack := runbase.Stack{
		TerragruntOptions: terragruntOptions,
	}

	runner := &Runner{
		Stack: &stack,
	}

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

		orderedUnits = append(orderedUnits, mod)
		unitsMap[cfg.Path] = mod
	}

	// cross-link dependencies units based on the discovered configurations
	for _, cfg := range discovered {
		unit := unitsMap[cfg.Path]

		for _, dependency := range cfg.Dependencies {
			path := dependency.Path
			if depUnit, ok := unitsMap[path]; ok {
				unit.Dependencies = append(unit.Dependencies, depUnit)
			} else {
				return nil, errors.Errorf("Dependency %s for unit %s not found in discovered units", path, unit.Path)
			}
		}
	}

	stack.Units = orderedUnits

	return runner.WithOptions(opts...), nil
}

// Run executes the stack according to TerragruntOptions and returns the first
// error (or a joined error) once execution is finished.
func (r *Runner) Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	terraformCmd := opts.TerraformCommand

	if opts.OutputFolder != "" {
		for _, u := range r.Stack.Units {
			planFile := u.OutputFile(l, opts)
			if err := os.MkdirAll(filepath.Dir(planFile), os.ModePerm); err != nil {
				return err
			}
		}
	}

	if util.ListContainsElement(config.TerraformCommandsNeedInput, terraformCmd) {
		opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-input=false", 1)
		r.syncTerraformCliArgs(l, opts)
	}

	var planDefer bool

	switch terraformCmd {
	case tf.CommandNameApply, tf.CommandNameDestroy:
		r.handleApplyDestroy(l, opts)
	case tf.CommandNameShow:
		r.handleShow(l, opts)
	case tf.CommandNamePlan:
		r.handlePlan()

		planDefer = true
	}

	if planDefer {
		defer r.summarizePlanAllErrors(l, r.planErrorBuffers)
	}

	taskRun := func(ctx context.Context, t *Task) Result {
		unitRunner := runbase.NewUnitRunner(t.Unit)
		err := unitRunner.Run(ctx, t.Unit.TerragruntOptions, r.Stack.Report)

		res := Result{TaskID: t.ID()}
		if err != nil {
			res.Err = err
			res.ExitCode = 1
		}

		return res
	}

	pool := NewRunnerPool(r.Stack.Units, taskRun, opts.Parallelism, opts.FailFast)

	results := pool.Run(ctx, l)

	var allErrs []error

	for _, res := range results {
		if res.Err != nil {
			allErrs = append(allErrs, res.Err)
		}
	}

	if len(allErrs) > 0 {
		return errors.Join(allErrs...)
	}

	return nil
}

// handleApplyDestroy handles logic for apply and destroy commands.
func (r *Runner) handleApplyDestroy(l log.Logger, opts *options.TerragruntOptions) {
	if opts.RunAllAutoApprove {
		opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-auto-approve", 1)
	}

	r.syncTerraformCliArgs(l, opts)
}

// handleShow handles logic for show command.
func (r *Runner) handleShow(l log.Logger, opts *options.TerragruntOptions) {
	r.syncTerraformCliArgs(l, opts)
}

// handlePlan handles logic for plan command, including error buffer setup and summary.
func (r *Runner) handlePlan() {
	r.planErrorBuffers = make([]bytes.Buffer, len(r.Stack.Units))
	for i, u := range r.Stack.Units {
		u.TerragruntOptions.ErrWriter = io.MultiWriter(&r.planErrorBuffers[i], u.TerragruntOptions.ErrWriter)
	}
}

func (r *Runner) LogUnitDeployOrder(l log.Logger, terraformCommand string) error {
	outStr := fmt.Sprintf("The runner-pool runner at %s will be processed in the following order for command %s:\n", r.Stack.TerragruntOptions.WorkingDir, terraformCommand)
	for _, unit := range r.Stack.Units {
		outStr += fmt.Sprintf("Unit %s\n", unit.Path)
	}

	l.Info(outStr)

	return nil
}

func (r *Runner) JSONUnitDeployOrder(terraformCommand string) (string, error) {
	orderedUnits := make([]string, 0, len(r.Stack.Units))
	for _, unit := range r.Stack.Units {
		orderedUnits = append(orderedUnits, unit.Path)
	}

	j, err := json.MarshalIndent(orderedUnits, "", "  ")
	if err != nil {
		return "", err
	}

	return string(j), nil
}

func (r *Runner) ListStackDependentUnits() map[string][]string {
	dependentUnits := make(map[string][]string)

	for _, unit := range r.Stack.Units {
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
func (r *Runner) syncTerraformCliArgs(l log.Logger, opts *options.TerragruntOptions) {
	for _, unit := range r.Stack.Units {
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
// remote states. `terraform plan` will fail if it tries to access remote State from dependencies and the plan
// has never been applied on the dependency.
func (r *Runner) summarizePlanAllErrors(l log.Logger, errorStreams []bytes.Buffer) {
	for i, errorStream := range errorStreams {
		output := errorStream.String()

		if len(output) == 0 {
			// We get Empty buffer if runner execution completed without errors, so skip that to avoid logging too much
			continue
		}

		if strings.Contains(output, "Error running plan:") && strings.Contains(output, ": Resource 'data.terraform_remote_state.") {
			var dependenciesMsg string
			if len(r.Stack.Units[i].Dependencies) > 0 {
				dependenciesMsg = fmt.Sprintf(" contains dependencies to %v and", r.Stack.Units[i].Config.Dependencies.Paths)
			}

			l.Infof("%v%v refers to remote State "+
				"you may have to apply your changes in the dependencies prior running terragrunt run --all plan.\n",
				r.Stack.Units[i].Path,
				dependenciesMsg,
			)
		}
	}
}

// WithOptions updates the stack with the provided options.
func (r *Runner) WithOptions(opts ...runbase.Option) *Runner {
	for _, opt := range opts {
		opt(r)
	}

	return r
}

func (r *Runner) GetStack() *runbase.Stack {
	return r.Stack
}

// SetTerragruntConfig sets the report for the stack.
func (r *Runner) SetTerragruntConfig(config *config.TerragruntConfig) {
	r.Stack.ChildTerragruntConfig = config
}

// SetParseOptions sets the report for the stack.
func (r *Runner) SetParseOptions(parserOptions []hclparse.Option) {
	r.Stack.ParserOptions = parserOptions
}

// SetReport sets the report for the stack.
func (r *Runner) SetReport(report *report.Report) {
	r.Stack.Report = report
}
