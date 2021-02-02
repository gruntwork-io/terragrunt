package configstack

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// Represents a stack of Terraform modules (i.e. folders with Terraform templates) that you can "spin up" or
// "spin down" in a single command
type Stack struct {
	Path    string
	Modules []*TerraformModule
}

// Render this stack as a human-readable string
func (stack *Stack) String() string {
	modules := []string{}
	for _, module := range stack.Modules {
		modules = append(modules, fmt.Sprintf("  => %s", module.String()))
	}
	sort.Strings(modules)
	return fmt.Sprintf("Stack at %s:\n%s", stack.Path, strings.Join(modules, "\n"))
}

// Graph creates a graphviz representation of the modules
func (stack *Stack) Graph(terragruntOptions *options.TerragruntOptions) {
	WriteDot(terragruntOptions.Writer, terragruntOptions, stack.Modules)
}

func (stack *Stack) Run(terragruntOptions *options.TerragruntOptions) error {
	stackCmd := terragruntOptions.TerraformCommand

	// For any command that needs input, run in non-interactive mode to avoid cominglint stdin across multiple
	// concurrent runs.
	if util.ListContainsElement(config.TERRAFORM_COMMANDS_NEED_INPUT, stackCmd) {
		terragruntOptions.TerraformCliArgs = append(terragruntOptions.TerraformCliArgs, "-input=false")
		stack.syncTerraformCliArgs(terragruntOptions)
	}

	// For apply and destroy, run with auto-approve due to the co-mingling of the prompts. This is not ideal, but until
	// we have a better way of handling interactivity with run-all, we take the evil of having a global prompt (managed
	// in cli/cli_app.go) be the gate keeper.
	switch stackCmd {
	case "apply", "destroy":
		terragruntOptions.TerraformCliArgs = append(terragruntOptions.TerraformCliArgs, "-auto-approve")
		stack.syncTerraformCliArgs(terragruntOptions)
	}

	if stackCmd == "plan" {
		// We capture the out stream for each module
		errorStreams := make([]bytes.Buffer, len(stack.Modules))
		for n, module := range stack.Modules {
			module.TerragruntOptions.ErrWriter = &errorStreams[n]
		}
		defer stack.summarizePlanAllErrors(terragruntOptions, errorStreams)
	}

	if terragruntOptions.IgnoreDependencyOrder {
		return RunModulesIgnoreOrder(stack.Modules, terragruntOptions.Parallelism)
	} else if stackCmd == "destroy" {
		return RunModulesReverseOrder(stack.Modules, terragruntOptions.Parallelism)
	} else {
		return RunModules(stack.Modules, terragruntOptions.Parallelism)
	}
}

// We inspect the error streams to give an explicit message if the plan failed because there were references to
// remote states. `terraform plan` will fail if it tries to access remote state from dependencies and the plan
// has never been applied on the dependency.
func (stack *Stack) summarizePlanAllErrors(terragruntOptions *options.TerragruntOptions, errorStreams []bytes.Buffer) {
	for i, errorStream := range errorStreams {
		output := errorStream.String()
		terragruntOptions.Logger.Println(output)
		if strings.Contains(output, "Error running plan:") {
			if strings.Contains(output, ": Resource 'data.terraform_remote_state.") {
				var dependenciesMsg string
				if len(stack.Modules[i].Dependencies) > 0 {
					dependenciesMsg = fmt.Sprintf(" contains dependencies to %v and", stack.Modules[i].Config.Dependencies.Paths)
				}
				terragruntOptions.Logger.Printf("%v%v refers to remote state "+
					"you may have to apply your changes in the dependencies prior running terragrunt plan-all.\n",
					stack.Modules[i].Path,
					dependenciesMsg,
				)
			}
		}
	}
}

// Return an error if there is a dependency cycle in the modules of this stack.
func (stack *Stack) CheckForCycles() error {
	return CheckForCycles(stack.Modules)
}

// Find all the Terraform modules in the subfolders of the working directory of the given TerragruntOptions and
// assemble them into a Stack object that can be applied or destroyed in a single command
func FindStackInSubfolders(terragruntOptions *options.TerragruntOptions) (*Stack, error) {
	terragruntConfigFiles, err := config.FindConfigFilesInPath(terragruntOptions.WorkingDir, terragruntOptions)
	if err != nil {
		return nil, err
	}

	howThesePathsWereFound := fmt.Sprintf("Terragrunt config file found in a subdirectory of %s", terragruntOptions.WorkingDir)
	return createStackForTerragruntConfigPaths(terragruntOptions.WorkingDir, terragruntConfigFiles, terragruntOptions, howThesePathsWereFound)
}

// Sync the TerraformCliArgs for each module in the stack to match the provided terragruntOptions struct.
func (stack *Stack) syncTerraformCliArgs(terragruntOptions *options.TerragruntOptions) {
	for _, module := range stack.Modules {
		module.TerragruntOptions.TerraformCliArgs = terragruntOptions.TerraformCliArgs
	}
}

// Find all the Terraform modules in the folders that contain the given Terragrunt config files and assemble those
// modules into a Stack object that can be applied or destroyed in a single command
func createStackForTerragruntConfigPaths(path string, terragruntConfigPaths []string, terragruntOptions *options.TerragruntOptions, howThesePathsWereFound string) (*Stack, error) {
	if len(terragruntConfigPaths) == 0 {
		return nil, errors.WithStackTrace(NoTerraformModulesFound)
	}

	modules, err := ResolveTerraformModules(terragruntConfigPaths, terragruntOptions, howThesePathsWereFound)
	if err != nil {
		return nil, err
	}

	stack := &Stack{Path: path, Modules: modules}
	if err := stack.CheckForCycles(); err != nil {
		return nil, err
	}

	return stack, nil
}

// Custom error types

var NoTerraformModulesFound = fmt.Errorf("Could not find any subfolders with Terragrunt configuration files")

type DependencyCycle []string

func (err DependencyCycle) Error() string {
	return fmt.Sprintf("Found a dependency cycle between modules: %s", strings.Join([]string(err), " -> "))
}
