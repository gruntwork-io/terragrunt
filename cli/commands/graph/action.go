package graph

import (
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
)

const (
	stackCommand = "destroy"
)

func Run(opts *options.TerragruntOptions) error {

	//opts.OriginalTerraformCommand = stackCommand
	//opts.TerraformCommand = stackCommand
	//opts.TerraformCliArgs = []string{stackCommand}

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

	// for each element from slice copy respective slice from map
	// loop while are changes in copy
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
