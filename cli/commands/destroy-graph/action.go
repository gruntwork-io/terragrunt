package destroy_graph

import (
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"

	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
)

const (
	stackCommand = "destroy"
)

func Run(opts *options.TerragruntOptions) error {

	opts.OriginalTerraformCommand = stackCommand
	opts.TerraformCommand = stackCommand
	opts.TerraformCliArgs = []string{stackCommand}

	// consider root for graph identification passed destroy-graph-root argument
	rootDir := opts.DestroyGraphRoot

	// if destroy-graph-root is empty, use git to find top level dir.
	// may cause issues if in the same repo exist unrelated modules which will generate errors when scanning.
	if rootDir == "" {
		gitRoot, err := shell.GitTopLevelDir(opts, opts.WorkingDir)
		if err != nil {
			return err
		}
		rootDir = gitRoot
	}

	rootOptions := opts.Clone(rootDir)
	rootOptions.WorkingDir = rootDir

	stack, err := configstack.FindStackInSubfolders(rootOptions, nil)
	if err != nil {
		return err
	}

	// filter dependencies to have keep only dependencies with working dir
	filterDependencies(stack, opts.WorkingDir)

	return runall.RunAllOnStack(opts, stack)
}

// isDependentOn checks if the workDir is directly or indirectly dependent on the given module
func isDependentOn(module *configstack.TerraformModule, workDir string, allModules map[string]*configstack.TerraformModule, visited map[string]bool) bool {
	if module.Path == workDir {
		return true
	}
	if visited[module.Path] {
		return false
	}
	visited[module.Path] = true

	// Check if any of the dependencies of this module lead to the workDir
	for _, dep := range module.Dependencies {
		if isDependentOn(dep, workDir, allModules, visited) {
			return true
		}
	}

	// Check if this module is a dependency of any other module that leads to the workDir
	for _, mod := range allModules {
		for _, dep := range mod.Dependencies {
			if dep.Path == module.Path && isDependentOn(mod, workDir, allModules, visited) {
				return true
			}
		}
	}

	return false
}

// filterDependencies updates the stack to only include modules that are dependent on the given path
func filterDependencies(stack *configstack.Stack, workDir string) {
	var filteredModules []*configstack.TerraformModule
	allModules := make(map[string]*configstack.TerraformModule)

	for _, module := range stack.Modules {
		allModules[module.Path] = module
	}

	for _, module := range stack.Modules {
		visited := make(map[string]bool)
		if isDependentOn(module, workDir, allModules, visited) {
			filteredModules = append(filteredModules, module)
			module.FlagExcluded = false
		} else {
			module.FlagExcluded = true
		}
	}
}
