package configstack

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// RunnerPoolStack implements the Stack interface for runner pool execution.
type RunnerPoolStack struct {
	modules           TerraformModules
	report            *report.Report
	parserOptions     []hclparse.Option
	terragruntOptions *options.TerragruntOptions
	outputMu          sync.Mutex
	childConfig       *config.TerragruntConfig
}

// NewRunnerPoolStack creates a new stack from discovered modules.
func NewRunnerPoolStack(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, discovered discovery.DiscoveredConfigs) (*RunnerPoolStack, error) {
	modulesMap := make(TerraformModulesMap)

	stack := &RunnerPoolStack{
		terragruntOptions: terragruntOptions,
		parserOptions:     config.DefaultParserOptions(l, terragruntOptions),
	}

	var terragruntConfigPaths []string
	for _, cfg := range discovered {
		configPath := config.GetDefaultConfigPath(cfg.Path)
		if cfg.Parsed == nil {
			// Skip configurations that could not be parsed
			l.Warnf("Skipping module at %s due to parse error: %s", cfg.Path, configPath)
			continue
		}
		modLogger, modOpts, err := terragruntOptions.CloneWithConfigPath(l, configPath)
		if err != nil {
			l.Warnf("Skipping module at %s due to error cloning options: %s", cfg.Path, err)
			continue // skip on error
		}
		mod := &TerraformModule{
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

	stack.modules = linkedModules

	return stack, nil
}

func (stack *RunnerPoolStack) SetReport(report *report.Report) {
	stack.report = report
}

func (stack *RunnerPoolStack) GetReport() *report.Report {
	return stack.report
}

func (stack *RunnerPoolStack) String() string {
	modules := []string{}
	for _, module := range stack.modules {
		modules = append(modules, "  => "+module.String())
	}
	// Sort for deterministic output
	sort.Strings(modules)
	return fmt.Sprintf("Stack at %s:\n%s", stack.terragruntOptions.WorkingDir, strings.Join(modules, "\n"))
}

func (stack *RunnerPoolStack) LogModuleDeployOrder(l log.Logger, terraformCommand string) error {
	order := NormalOrder
	if terraformCommand == "destroy" {
		order = ReverseOrder
	}
	runningModules, err := stack.modules.ToRunningModules(order, stack.report, stack.terragruntOptions)
	if err != nil {
		return err
	}
	// Flatten the modules in run order
	orderedModules := make([]*TerraformModule, 0, len(runningModules))
	for _, module := range runningModules {
		if !module.FlagExcluded {
			orderedModules = append(orderedModules, module.Module)
		}
	}
	outStr := fmt.Sprintf("The stack at %s will be processed in the following order for command %s:\n", stack.terragruntOptions.WorkingDir, terraformCommand)
	for _, module := range orderedModules {
		outStr += fmt.Sprintf("Module %s\n", module.Path)
	}
	l.Info(outStr)
	return nil
}

func (stack *RunnerPoolStack) JSONModuleDeployOrder(terraformCommand string) (string, error) {
	order := NormalOrder
	if terraformCommand == "destroy" {
		order = ReverseOrder
	}
	runningModules, err := stack.modules.ToRunningModules(order, stack.report, stack.terragruntOptions)
	if err != nil {
		return "", err
	}
	orderedModules := make([]string, 0, len(runningModules))
	for _, module := range runningModules {
		if !module.FlagExcluded {
			orderedModules = append(orderedModules, module.Module.Path)
		}
	}
	j, err := json.MarshalIndent(orderedModules, "", "  ")
	if err != nil {
		return "", err
	}
	return string(j), nil
}

func (stack *RunnerPoolStack) Graph(l log.Logger, opts *options.TerragruntOptions) {
	err := stack.modules.WriteDot(l, opts.Writer, opts)
	if err != nil {
		l.Warnf("Failed to graph dot: %v", err)
	}
}

func (stack *RunnerPoolStack) Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
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
		// No syncTerraformCliArgs for runner pool, but can be added if needed
	}
	switch {
	case opts.IgnoreDependencyOrder:
		return stack.modules.RunModulesIgnoreOrder(ctx, opts, stack.report, opts.Parallelism)
	case stackCmd == "destroy":
		return stack.modules.RunModulesReverseOrder(ctx, opts, stack.report, opts.Parallelism)
	default:
		return stack.modules.RunModules(ctx, opts, stack.report, opts.Parallelism)
	}
}

func (stack *RunnerPoolStack) GetModuleRunGraph(terraformCommand string) ([]TerraformModules, error) {
	var order DependencyOrder
	if terraformCommand == "destroy" {
		order = ReverseOrder
	} else {
		order = NormalOrder
	}
	runningModules, err := stack.modules.ToRunningModules(order, stack.report, stack.terragruntOptions)
	if err != nil {
		return nil, err
	}
	const maxDepth = 1000
	groups := runningModules.toTerraformModuleGroups(maxDepth)
	return groups, nil
}

func (stack *RunnerPoolStack) ListStackDependentModules() map[string][]string {
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

func (stack *RunnerPoolStack) Modules() TerraformModules {
	return stack.modules
}

func (stack *RunnerPoolStack) FindModuleByPath(path string) *TerraformModule {
	for _, module := range stack.modules {
		if module.Path == path {
			return module
		}
	}
	return nil
}

func (stack *RunnerPoolStack) SetTerragruntConfig(config *config.TerragruntConfig) {
	stack.childConfig = config
}

func (stack *RunnerPoolStack) GetTerragruntConfig() *config.TerragruntConfig {
	return stack.childConfig
}

func (stack *RunnerPoolStack) SetParseOptions(parserOptions []hclparse.Option) {
	stack.parserOptions = parserOptions
}

func (stack *RunnerPoolStack) GetParseOptions() []hclparse.Option {
	return stack.parserOptions
}

func (stack *RunnerPoolStack) SetModules(modules TerraformModules) {
	stack.modules = modules
}

func (stack *RunnerPoolStack) Lock() {
	stack.outputMu.Lock()
}

func (stack *RunnerPoolStack) Unlock() {
	stack.outputMu.Unlock()
}
