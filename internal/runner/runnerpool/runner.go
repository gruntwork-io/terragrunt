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

// NewRunnerPoolStack creates a new stack from discovered modules.
func NewRunnerPoolStack(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, discovered discovery.DiscoveredConfigs, opts ...runbase.Option) (runbase.StackRunner, error) {
	modulesMap := make(runbase.UnitsMap, len(discovered))

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

		modulesMap[cfg.Path] = mod
	}

	canonicalTerragruntConfigPaths, err := util.CanonicalPaths(terragruntConfigPaths, ".")
	if err != nil {
		return nil, err
	}

	linkedUnits, err := modulesMap.CrossLinkDependencies(canonicalTerragruntConfigPaths)
	if err != nil {
		return nil, err
	}
	// Reorder linkedUnits to match the order of canonicalTerragruntConfigPaths
	orderedModules := make(runbase.Units, 0, len(canonicalTerragruntConfigPaths))
	pathToModule := make(map[string]*runbase.Unit)

	for _, m := range linkedUnits {
		pathToModule[config.GetDefaultConfigPath(m.Path)] = m
	}

	for _, configPath := range canonicalTerragruntConfigPaths {
		if m, ok := pathToModule[configPath]; ok {
			orderedModules = append(orderedModules, m)
		} else {
			l.Warnf("Unit for config path %s not found in linked units", configPath)
		}
	}

	stack.Units = orderedModules

	return runner.WithOptions(opts...), nil
}

func (runner *Runner) String() string {
	modules := []string{}
	for _, module := range runner.Stack.Units {
		modules = append(modules, "  => "+module.String())
	}

	return fmt.Sprintf("Stack at %s:\n%s", runner.Stack.TerragruntOptions.WorkingDir, strings.Join(modules, "\n"))
}

func (runner *Runner) LogModuleDeployOrder(l log.Logger, terraformCommand string) error {
	outStr := fmt.Sprintf("The runner-pool runner at %s will be processed in the following order for command %s:\n", runner.Stack.TerragruntOptions.WorkingDir, terraformCommand)
	for _, module := range runner.Stack.Units {
		outStr += fmt.Sprintf("Unit %s\n", module.Path)
	}

	l.Info(outStr)

	return nil
}

func (runner *Runner) JSONModuleDeployOrder(terraformCommand string) (string, error) {
	orderedModules := make([]string, 0, len(runner.Stack.Units))
	for _, module := range runner.Stack.Units {
		orderedModules = append(orderedModules, module.Path)
	}

	j, err := json.MarshalIndent(orderedModules, "", "  ")
	if err != nil {
		return "", err
	}

	return string(j), nil
}

func (runner *Runner) Graph(l log.Logger, opts *options.TerragruntOptions) {
	err := runner.Stack.Units.WriteDot(l, opts.Writer, opts)
	if err != nil {
		l.Warnf("Failed to graph dot: %v", err)
	}
}

func (runner *Runner) Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	// Here will be implemented runner pool logic to run the modules concurrently.
	// Currently, implementation is in the sequential way.
	stackCmd := opts.TerraformCommand

	if opts.OutputFolder != "" {
		for _, module := range runner.Stack.Units {
			planFile := module.OutputFile(l, opts)
			planDir := filepath.Dir(planFile)

			if err := os.MkdirAll(planDir, os.ModePerm); err != nil {
				return err
			}
		}
	}

	if util.ListContainsElement(config.TerraformCommandsNeedInput, stackCmd) {
		opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-input=false", 1)
		runner.syncTerraformCliArgs(l, opts)
	}

	// configure CLI args to apply on the runner level
	switch stackCmd {
	case tf.CommandNameApply, tf.CommandNameDestroy:
		if opts.RunAllAutoApprove {
			opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-auto-approve", 1)
		}

		runner.syncTerraformCliArgs(l, opts)
	case tf.CommandNameShow:
		runner.syncTerraformCliArgs(l, opts)
	case tf.CommandNamePlan:
		errorStreams := make([]bytes.Buffer, len(runner.Stack.Units))

		for n, module := range runner.Stack.Units {
			module.TerragruntOptions.ErrWriter = io.MultiWriter(&errorStreams[n], module.TerragruntOptions.ErrWriter)
		}

		defer runner.summarizePlanAllErrors(l, errorStreams)
	}

	var errs []error

	// Run each module in the runner sequentially, convert each module to a running module, and run it.
	for _, module := range runner.Stack.Units {
		moduleToRun := runbase.NewUnitRunner(module)
		if err := moduleToRun.Run(ctx, module.TerragruntOptions, runner.Stack.Report); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (runner *Runner) ListStackDependentModules() map[string][]string {
	dependentModules := make(map[string][]string)

	for _, module := range runner.Stack.Units {
		if len(module.Dependencies) != 0 {
			for _, dep := range module.Dependencies {
				dependentModules[dep.Path] = util.RemoveDuplicatesFromList(append(dependentModules[dep.Path], module.Path))
			}
		}
	}

	for {
		noUpdates := true

		for module, dependents := range dependentModules {
			for _, dependent := range dependents {
				initialSize := len(dependentModules[module])
				list := util.RemoveDuplicatesFromList(append(dependentModules[module], dependentModules[dependent]...))
				list = util.RemoveElementFromList(list, module)
				dependentModules[module] = list

				if initialSize != len(dependentModules[module]) {
					noUpdates = false
				}
			}
		}

		if noUpdates {
			break
		}
	}

	return dependentModules
}

// FindModuleByPath finds a module by its path.
func (runner *Runner) FindModuleByPath(path string) *runbase.Unit {
	for _, module := range runner.Stack.Units {
		if module.Path == path {
			return module
		}
	}

	return nil
}

// Sync the TerraformCliArgs for each module in the stack to match the provided terragruntOptions struct.
func (runner *Runner) syncTerraformCliArgs(l log.Logger, opts *options.TerragruntOptions) {
	for _, module := range runner.Stack.Units {
		module.TerragruntOptions.TerraformCliArgs = make([]string, len(opts.TerraformCliArgs))
		copy(module.TerragruntOptions.TerraformCliArgs, opts.TerraformCliArgs)

		planFile := module.PlanFile(l, opts)
		if planFile != "" {
			l.Debugf("Using output file %s for unit %s", planFile, module.TerragruntOptions.TerragruntConfigPath)

			if module.TerragruntOptions.TerraformCommand == "plan" {
				// for plan command add -out=<file> to the terraform cli args
				module.TerragruntOptions.TerraformCliArgs = append(module.TerragruntOptions.TerraformCliArgs, "-out="+planFile)
			} else {
				module.TerragruntOptions.TerraformCliArgs = append(module.TerragruntOptions.TerraformCliArgs, planFile)
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
