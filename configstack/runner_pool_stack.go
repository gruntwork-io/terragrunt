package configstack

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
)

// RunnerPoolStack implements the Stack interface and represents a stack of Terraform modules discovered via the discovery package.
type RunnerPoolStack struct {
	report                *report.Report
	parserOptions         []hclparse.Option
	terragruntOptions     *options.TerragruntOptions
	childTerragruntConfig *config.TerragruntConfig
	modules               TerraformModules
	outputMu              sync.Mutex
}

// NewRunnerPoolStackWithDiscovery creates a new RunnerPoolStack from already discovered modules.
func NewRunnerPoolStackWithDiscovery(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, discovered discovery.DiscoveredConfigs) *RunnerPoolStack {
	modules := ConvertDiscoveredToModules(discovered, terragruntOptions, l)
	return &RunnerPoolStack{
		report:            nil,
		parserOptions:     config.DefaultParserOptions(l, terragruntOptions),
		terragruntOptions: terragruntOptions,
		modules:           modules,
	}
}

// ConvertDiscoveredToModules converts discovered configs to TerraformModules (basic placeholder logic).
func ConvertDiscoveredToModules(discovered discovery.DiscoveredConfigs, terragruntOptions *options.TerragruntOptions, l log.Logger) TerraformModules {
	modules := TerraformModules{}
	for _, d := range discovered {
		if d.Parsed == nil {
			continue
		}
		mod := &TerraformModule{
			Stack:             nil, // Will be set later if needed
			TerragruntOptions: terragruntOptions,
			Logger:            l,
			Path:              d.Path,
			Config:            *d.Parsed,
			Dependencies:      TerraformModules{}, // Not wired in placeholder
		}
		modules = append(modules, mod)
	}
	return modules
}

// String renders this stack as a human-readable string
func (stack *RunnerPoolStack) String() string {
	modules := []string{}
	for _, module := range stack.modules {
		modules = append(modules, "  => "+module.String())
	}
	sort.Strings(modules)
	return fmt.Sprintf("RunnerPoolStack at %s:\n%s", stack.terragruntOptions.WorkingDir, strings.Join(modules, "\n"))
}

// LogModuleDeployOrder logs the modules in deploy order (placeholder: just lists modules)
func (stack *RunnerPoolStack) LogModuleDeployOrder(l log.Logger, terraformCommand string) error {
	l.Info("[RunnerPoolStack] Module deploy order:")
	for _, module := range stack.modules {
		l.Info("- " + module.Path)
	}
	return nil
}

// JSONModuleDeployOrder returns a JSON string of module paths (placeholder)
func (stack *RunnerPoolStack) JSONModuleDeployOrder(terraformCommand string) (string, error) {
	paths := []string{}
	for _, module := range stack.modules {
		paths = append(paths, module.Path)
	}
	return fmt.Sprintf(`{"modules": ["%s"]}`, strings.Join(paths, `","`)), nil
}

// Graph outputs a placeholder graphviz dot (just module nodes)
func (stack *RunnerPoolStack) Graph(l log.Logger, opts *options.TerragruntOptions) {
	l.Info("digraph {\n" + strings.Join(stack.moduleNames(), "\n") + "\n}")
}

func (stack *RunnerPoolStack) moduleNames() []string {
	names := []string{}
	for _, m := range stack.modules {
		names = append(names, fmt.Sprintf("\t\"%s\";", m.Path))
	}
	return names
}

// Run is a placeholder that just logs the modules
func (stack *RunnerPoolStack) Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	l.Info("[RunnerPoolStack] Would run modules:")
	for _, module := range stack.modules {
		l.Info("- " + module.Path)
	}
	return nil
}

// GetModuleRunGraph returns a single group with all modules (placeholder)
func (stack *RunnerPoolStack) GetModuleRunGraph(terraformCommand string) ([]TerraformModules, error) {
	return []TerraformModules{stack.modules}, nil
}

// ListStackDependentModules builds a map with each module and its dependent modules.
func (stack *RunnerPoolStack) ListStackDependentModules() map[string][]string {
	dependentModules := make(map[string][]string)

	// Initial mapping: for each module, add itself as a dependent to its dependencies
	for _, module := range stack.modules {
		for _, dep := range module.Dependencies {
			dependentModules[dep.Path] = append(dependentModules[dep.Path], module.Path)
		}
	}

	// Floyd–Warshall style: propagate dependencies transitively
	for {
		noUpdates := true
		for module, dependents := range dependentModules {
			for _, dependent := range dependents {
				initialSize := len(dependentModules[module])
				// Merge without duplicates
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

// Modules returns the Terraform modules in the stack.
func (stack *RunnerPoolStack) Modules() TerraformModules {
	return stack.modules
}

// FindModuleByPath finds a module by its path.
func (stack *RunnerPoolStack) FindModuleByPath(path string) *TerraformModule {
	for _, module := range stack.modules {
		if module.Path == path {
			return module
		}
	}
	return nil
}

func (stack *RunnerPoolStack) SetTerragruntConfig(cfg *config.TerragruntConfig) {
	stack.childTerragruntConfig = cfg
}
func (stack *RunnerPoolStack) GetTerragruntConfig() *config.TerragruntConfig {
	return stack.childTerragruntConfig
}
func (stack *RunnerPoolStack) SetParseOptions(parserOptions []hclparse.Option) {
	stack.parserOptions = parserOptions
}
func (stack *RunnerPoolStack) GetParseOptions() []hclparse.Option { return stack.parserOptions }
func (stack *RunnerPoolStack) SetReport(report *report.Report)    { stack.report = report }
func (stack *RunnerPoolStack) GetReport() *report.Report          { return stack.report }
func (stack *RunnerPoolStack) Lock()                              { stack.outputMu.Lock() }
func (stack *RunnerPoolStack) Unlock()                            { stack.outputMu.Unlock() }
