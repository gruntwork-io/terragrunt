package configstack

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/sirupsen/logrus"
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

// LogModuleDeployOrder will log the modules that will be deployed by this operation, in the order that the operations
// happen. For plan and apply, the order will be bottom to top (dependencies first), while for destroy the order will be
// in reverse.
func (stack *Stack) LogModuleDeployOrder(logger *logrus.Entry, terraformCommand string) error {
	outStr := fmt.Sprintf("The stack at %s will be processed in the following order for command %s:\n", stack.Path, terraformCommand)
	runGraph, err := stack.getModuleRunGraph(terraformCommand)
	if err != nil {
		return err
	}
	for i, group := range runGraph {
		outStr += fmt.Sprintf("Group %d\n", i+1)
		for _, module := range group {
			outStr += fmt.Sprintf("- Module %s\n", module.Path)
		}
		outStr += "\n"
	}
	logger.Info(outStr)
	return nil
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
		// to support potential positional args in the args list, we append the input=false arg after the first element,
		// which is the target command.
		terragruntOptions.TerraformCliArgs = util.StringListInsert(terragruntOptions.TerraformCliArgs, "-input=false", 1)
		stack.syncTerraformCliArgs(terragruntOptions)
	}

	// For apply and destroy, run with auto-approve (unless explicitly disabled) due to the co-mingling of the prompts.
	// This is not ideal, but until we have a better way of handling interactivity with run-all, we take the evil of
	// having a global prompt (managed in cli/cli_app.go) be the gate keeper.
	switch stackCmd {
	case "apply", "destroy":
		// to support potential positional args in the args list, we append the input=false arg after the first element,
		// which is the target command.
		if terragruntOptions.RunAllAutoApprove {
			terragruntOptions.TerraformCliArgs = util.StringListInsert(terragruntOptions.TerraformCliArgs, "-auto-approve", 1)
		}
		stack.syncTerraformCliArgs(terragruntOptions)
	}

	if stackCmd == "plan" {
		// We capture the out stream for each module
		errorStreams := make([]bytes.Buffer, len(stack.Modules))
		for n, module := range stack.Modules {
			if !terragruntOptions.NonInteractive { // redirect output to ErrWriter in case of not NonInteractive mode
				module.TerragruntOptions.ErrWriter = io.MultiWriter(&errorStreams[n], module.TerragruntOptions.ErrWriter)
			} else {
				module.TerragruntOptions.ErrWriter = &errorStreams[n]
			}
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

		if len(output) == 0 {
			// We get empty buffer if stack execution completed without errors, so skip that to avoid logging too much
			continue
		}

		terragruntOptions.Logger.Infoln(output)
		if strings.Contains(output, "Error running plan:") {
			if strings.Contains(output, ": Resource 'data.terraform_remote_state.") {
				var dependenciesMsg string
				if len(stack.Modules[i].Dependencies) > 0 {
					dependenciesMsg = fmt.Sprintf(" contains dependencies to %v and", stack.Modules[i].Config.Dependencies.Paths)
				}
				terragruntOptions.Logger.Infof("%v%v refers to remote state "+
					"you may have to apply your changes in the dependencies prior running terragrunt run-all plan.\n",
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
func FindStackInSubfolders(terragruntOptions *options.TerragruntOptions, childTerragruntConfig *config.TerragruntConfig) (*Stack, error) {
	terragruntConfigFiles, err := config.FindConfigFilesInPath(terragruntOptions.WorkingDir, terragruntOptions)
	if err != nil {
		return nil, err
	}

	howThesePathsWereFound := fmt.Sprintf("Terragrunt config file found in a subdirectory of %s", terragruntOptions.WorkingDir)
	return createStackForTerragruntConfigPaths(terragruntOptions.WorkingDir, terragruntConfigFiles, terragruntOptions, childTerragruntConfig, howThesePathsWereFound)
}

// Sync the TerraformCliArgs for each module in the stack to match the provided terragruntOptions struct.
func (stack *Stack) syncTerraformCliArgs(terragruntOptions *options.TerragruntOptions) {
	for _, module := range stack.Modules {
		module.TerragruntOptions.TerraformCliArgs = terragruntOptions.TerraformCliArgs
	}
}

// getModuleRunGraph converts the module list to a graph that shows the order in which the modules will be
// applied/destroyed. The return structure is a list of lists, where the nested list represents modules that can be
// deployed concurrently, and the outer list indicates the order. This will only include those modules that do NOT have
// the exclude flag set.
func (stack *Stack) getModuleRunGraph(terraformCommand string) ([][]*TerraformModule, error) {
	var moduleRunGraph map[string]*runningModule
	var graphErr error
	switch terraformCommand {
	case "destroy":
		moduleRunGraph, graphErr = toRunningModules(stack.Modules, ReverseOrder)
	default:
		moduleRunGraph, graphErr = toRunningModules(stack.Modules, NormalOrder)
	}
	if graphErr != nil {
		return nil, graphErr
	}

	// Set maxDepth for the graph so that we don't get stuck in an infinite loop.
	const maxDepth = 1000

	// Walk the graph in run order, capturing which groups will run at each iteration. In each iteration, this pops out
	// the modules that have no dependencies and captures that as a run group.
	groups := [][]*TerraformModule{}
	for len(moduleRunGraph) > 0 && len(groups) < maxDepth {
		currentIterationDeploy := []*TerraformModule{}

		// next tracks which modules are being deferred to a later run.
		next := map[string]*runningModule{}
		// removeDep tracks which modules are run in the current iteration so that they need to be removed in the
		// dependency list for the next iteration. This is separately tracked from currentIterationDeploy for
		// convenience: this tracks the map key of the Dependencies attribute.
		removeDep := []string{}

		// Iterate the modules, looking for those that have no dependencies and select them for "running". In the
		// process, track those that still need to run in a separate map for further processing.
		for path, module := range moduleRunGraph {
			// Anything that is already applied is culled from the graph when running, so we ignore them here as well.
			if module.Module.AssumeAlreadyApplied {
				removeDep = append(removeDep, path)
			} else if len(module.Dependencies) == 0 {
				currentIterationDeploy = append(currentIterationDeploy, module.Module)
				removeDep = append(removeDep, path)
			} else {
				next[path] = module
			}
		}

		// Go through the remaining module and remove the dependencies that were selected to run in this curent
		// iteration.
		for _, module := range next {
			for _, path := range removeDep {
				_, hasDep := module.Dependencies[path]
				if hasDep {
					delete(module.Dependencies, path)
				}
			}
		}

		// Sort the group by path so that it is easier to read and test.
		sort.Slice(
			currentIterationDeploy,
			func(i, j int) bool {
				return currentIterationDeploy[i].Path < currentIterationDeploy[j].Path
			},
		)

		// Finally, update the trackers so that the next iteration runs.
		moduleRunGraph = next
		if len(currentIterationDeploy) > 0 {
			groups = append(groups, currentIterationDeploy)
		}
	}
	return groups, nil
}

// Find all the Terraform modules in the folders that contain the given Terragrunt config files and assemble those
// modules into a Stack object that can be applied or destroyed in a single command
func createStackForTerragruntConfigPaths(path string, terragruntConfigPaths []string, terragruntOptions *options.TerragruntOptions, childTerragruntConfig *config.TerragruntConfig, howThesePathsWereFound string) (*Stack, error) {
	if len(terragruntConfigPaths) == 0 {
		return nil, errors.WithStackTrace(NoTerraformModulesFound)
	}

	modules, err := ResolveTerraformModules(terragruntConfigPaths, terragruntOptions, childTerragruntConfig, howThesePathsWereFound)
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
