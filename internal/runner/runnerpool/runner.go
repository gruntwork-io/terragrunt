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

	"github.com/gruntwork-io/terragrunt/internal/runner/common"

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
	Stack *common.Stack
}

// NewRunnerPoolStack creates a new stack from discovered modules.
func NewRunnerPoolStack(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, discovered discovery.DiscoveredConfigs) (*Runner, error) {
	modulesMap := make(common.UnitsMap, len(discovered))

	stack := &Runner{
		terragruntOptions: terragruntOptions,
		parserOptions:     config.DefaultParserOptions(l, terragruntOptions),
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

		mod := &config.Unit{
			Stack:             stack,
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

	linkedModules, err := modulesMap.crosslinkDependencies(canonicalTerragruntConfigPaths)
	if err != nil {
		return nil, err
	}
	// Reorder linkedModules to match the order of canonicalTerragruntConfigPaths
	orderedModules := make(config.Units, 0, len(canonicalTerragruntConfigPaths))
	pathToModule := make(map[string]*config.Unit)

	for _, m := range linkedModules {
		pathToModule[config.GetDefaultConfigPath(m.Path)] = m
	}

	for _, configPath := range canonicalTerragruntConfigPaths {
		if m, ok := pathToModule[configPath]; ok {
			orderedModules = append(orderedModules, m)
		} else {
			l.Warnf("Unit for config path %s not found in linked units", configPath)
		}
	}

	stack.modules = orderedModules

	return stack, nil
}

func (stack *Runner) String() string {
	modules := []string{}
	for _, module := range stack.modules {
		modules = append(modules, "  => "+module.String())
	}

	return fmt.Sprintf("Stack at %s:\n%s", stack.terragruntOptions.WorkingDir, strings.Join(modules, "\n"))
}

func (stack *Runner) LogModuleDeployOrder(l log.Logger, terraformCommand string) error {
	outStr := fmt.Sprintf("The runner-pool stack at %s will be processed in the following order for command %s:\n", stack.terragruntOptions.WorkingDir, terraformCommand)
	for _, module := range stack.modules {
		outStr += fmt.Sprintf("Unit %s\n", module.Path)
	}

	l.Info(outStr)

	return nil
}

func (stack *Runner) JSONModuleDeployOrder(terraformCommand string) (string, error) {
	orderedModules := make([]string, 0, len(stack.modules))
	for _, module := range stack.modules {
		orderedModules = append(orderedModules, module.Path)
	}

	j, err := json.MarshalIndent(orderedModules, "", "  ")
	if err != nil {
		return "", err
	}

	return string(j), nil
}

func (stack *Runner) Graph(l log.Logger, opts *options.TerragruntOptions) {
	err := stack.modules.WriteDot(l, opts.Writer, opts)
	if err != nil {
		l.Warnf("Failed to graph dot: %v", err)
	}
}

func (stack *Runner) Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	// Here will be implemented runner pool logic to run the modules concurrently.
	// Currently, implementation is in the sequential way.
	stackCmd := opts.TerraformCommand

	if opts.OutputFolder != "" {
		for _, module := range stack.modules {
			planFile := module.outputFile(l, opts)
			planDir := filepath.Dir(planFile)

			if err := os.MkdirAll(planDir, os.ModePerm); err != nil {
				return err
			}
		}
	}

	if util.ListContainsElement(config.TerraformCommandsNeedInput, stackCmd) {
		opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-input=false", 1)
		stack.syncTerraformCliArgs(l, opts)
	}

	// configure CLI args to apply on the stack level
	switch stackCmd {
	case tf.CommandNameApply, tf.CommandNameDestroy:
		if opts.RunAllAutoApprove {
			opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-auto-approve", 1)
		}

		stack.syncTerraformCliArgs(l, opts)
	case tf.CommandNameShow:
		stack.syncTerraformCliArgs(l, opts)
	case tf.CommandNamePlan:
		errorStreams := make([]bytes.Buffer, len(stack.modules))

		for n, module := range stack.modules {
			module.TerragruntOptions.ErrWriter = io.MultiWriter(&errorStreams[n], module.TerragruntOptions.ErrWriter)
		}

		defer stack.summarizePlanAllErrors(l, errorStreams)
	}

	var errs []error

	// Run each module in the stack sequentially, convert each module to a running module, and run it.
	for _, module := range stack.modules {
		moduleToRun := configstack.newRunningModule(module)
		if err := moduleToRun.runNow(ctx, module.TerragruntOptions, stack.report); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (stack *Runner) GetModuleRunGraph(terraformCommand string) ([]configstack2.TerraformModules, error) {
	groups := make([]configstack2.TerraformModules, 0, len(stack.modules))
	for _, module := range stack.modules {
		groups = append(groups, configstack2.TerraformModules{module})
	}

	return groups, nil
}

func (stack *Runner) ListStackDependentModules() map[string][]string {
	dependentModules := make(map[string][]string)

	for _, module := range stack.modules {
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

func (stack *Runner) FindModuleByPath(path string) *configstack2.Unit {
	for _, module := range stack.modules {
		if module.Path == path {
			return module
		}
	}

	return nil
}

// Sync the TerraformCliArgs for each module in the stack to match the provided terragruntOptions struct.
func (stack *Runner) syncTerraformCliArgs(l log.Logger, opts *options.TerragruntOptions) {
	for _, module := range stack.modules {
		module.TerragruntOptions.TerraformCliArgs = make([]string, len(opts.TerraformCliArgs))
		copy(module.TerragruntOptions.TerraformCliArgs, opts.TerraformCliArgs)

		planFile := module.planFile(l, opts)
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
func (stack *Runner) summarizePlanAllErrors(l log.Logger, errorStreams []bytes.Buffer) {
	for i, errorStream := range errorStreams {
		output := errorStream.String()

		if len(output) == 0 {
			// We get empty buffer if stack execution completed without errors, so skip that to avoid logging too much
			continue
		}

		if strings.Contains(output, "Error running plan:") && strings.Contains(output, ": Resource 'data.terraform_remote_state.") {
			var dependenciesMsg string
			if len(stack.modules[i].Dependencies) > 0 {
				dependenciesMsg = fmt.Sprintf(" contains dependencies to %v and", stack.modules[i].Config.Dependencies.Paths)
			}

			l.Infof("%v%v refers to remote state "+
				"you may have to apply your changes in the dependencies prior running terragrunt run --all plan.\n",
				stack.modules[i].Path,
				dependenciesMsg,
			)
		}
	}
}

func (stack *Runner) SetReport(report *report.Report) {
	stack.report = report
}

func (stack *Runner) GetReport() *report.Report {
	return stack.report
}

func (stack *Runner) SetTerragruntConfig(config *config.TerragruntConfig) {
	stack.childConfig = config
}

func (stack *Runner) GetTerragruntConfig() *config.TerragruntConfig {
	return stack.childConfig
}

func (stack *Runner) SetParseOptions(parserOptions []hclparse.Option) {
	stack.parserOptions = parserOptions
}

func (stack *Runner) GetParseOptions() []hclparse.Option {
	return stack.parserOptions
}

func (stack *Runner) SetModules(modules config.Units) {
	stack.modules = modules
}

func (stack *Runner) Lock() {
	stack.outputMu.Lock()
}

func (stack *Runner) Unlock() {
	stack.outputMu.Unlock()
}

func (stack *Runner) Modules() config.Units {
	return stack.modules
}
