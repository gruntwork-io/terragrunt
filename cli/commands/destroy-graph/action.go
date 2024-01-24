package destroy_graph

import (
	"fmt"

	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"

	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
)

func Run(opts *options.TerragruntOptions) error {

	opts.OriginalTerraformCommand = "destroy"
	opts.TerraformCommand = "destroy"
	opts.TerraformCliArgs = []string{"destroy"}
	rootDir := opts.DestroyGraphRoot

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

	filterDependencies(stack, opts.WorkingDir)

	fmt.Printf("%v\n", stack)

	fmt.Printf("modules: \n%v\n", stack.Modules)

	return runall.RunAllOnStack(opts, stack)
}

// isDependentOn checks if the module is directly or indirectly dependent on the given path
func isDependentOn(module *configstack.TerraformModule, path string, visited map[*configstack.TerraformModule]bool) bool {
	if module.Path == path {
		return true
	}
	if visited[module] {
		return false
	}
	visited[module] = true

	for _, dep := range module.Dependencies {
		if isDependentOn(dep, path, visited) {
			return true
		}
	}
	return false
}

// filterDependencies updates the stack to only include modules that are dependent on the given path
func filterDependencies(stack *configstack.Stack, workDir string) {
	var filteredModules []*configstack.TerraformModule
	for _, module := range stack.Modules {
		visited := make(map[*configstack.TerraformModule]bool)
		if isDependentOn(module, workDir, visited) {
			filteredModules = append(filteredModules, module)
		}
	}
	stack.Modules = filteredModules
}
