package graph

import (
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
)

func Run(opts *options.TerragruntOptions) error {
	// consider root for graph identification passed destroy-graph-root argument
	rootDir := opts.GraphRoot

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

// filterDependencies updates the stack to only include modules that are dependent on the given path
func filterDependencies(stack *configstack.Stack, workDir string) {

	// build map of dependent modules
	// module path -> list of dependent modules
	var dependentModules = make(map[string][]string)

	// build initial mapping of dependent modules
	for _, module := range stack.Modules {

		if len(module.Dependencies) != 0 {
			for _, dep := range module.Dependencies {
				dependentModules[dep.Path] = util.RemoveDuplicatesFromList(append(dependentModules[dep.Path], module.Path))
			}
		}
	}

	// simple implementation Floyd–Warshall algorithm
	// loop copying all dependencies from dependent modules
	// Example:
	// Initial setup:
	// dependentModules["module1"] = ["module2", "module3"]
	// dependentModules["module2"] = ["module3"]
	// dependentModules["module3"] = ["module4"]
	// dependentModules["module4"] = ["module5"]

	// After first iteration: (added to module1 module4, added to module5 module 4, added to module3 module5)
	// dependentModules["module1"] = ["module2", "module3", "module4"]
	// dependentModules["module2"] = ["module3", "module4"]
	// dependentModules["module3"] = ["module4", "module5"]
	// dependentModules["module4"] = ["module5"]

	// After second iteration: (added to module1 module5, added to module2 module5)
	// dependentModules["module1"] = ["module2", "module3", "module4", "module5"]
	// dependentModules["module2"] = ["module3", "module4", "module5"]
	// dependentModules["module3"] = ["module4", "module5"]
	// dependentModules["module4"] = ["module5"]

	// Done, no more updates and in map we have all dependent modules for each module.

	for {
		noUpdates := true
		for module, dependents := range dependentModules {
			for _, dependent := range dependents {
				initialSize := len(dependentModules[module])
				// merge without duplicates
				dependentModules[module] = util.RemoveDuplicatesFromList(append(dependentModules[module], dependentModules[dependent]...))
				if initialSize != len(dependentModules[module]) {
					noUpdates = false
				}
			}
		}
		if noUpdates {
			break
		}
	}

	modulesToInclude := dependentModules[workDir]
	// workdir to list too
	modulesToInclude = append(modulesToInclude, workDir)

	// include from stack only elements from modulesToInclude
	for _, module := range stack.Modules {
		module.FlagExcluded = true
		if util.ListContainsElement(modulesToInclude, module.Path) {
			module.FlagExcluded = false
		}
	}
}
