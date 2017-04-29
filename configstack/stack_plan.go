package configstack

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"regexp"
	"strconv"
	"strings"
)

// The returned information for each module
type moduleResult struct {
	Module    TerraformModule
	Err       error
	Message   string
	NbChanges int
}

const CHANGE_EXIT_CODE = 2

var planResultRegex = regexp.MustCompile(`(\d+) to add, (\d+) to change, (\d+) to destroy.`)

func (stack *Stack) planWithSummary(terragruntOptions *options.TerragruntOptions) error {
	// We override the multi errors creator to use a specialized error type for plan
	// because error severity in plan is not standard (i.e. exit code 2 is less significant that exit code 1).
	CreateMultiErrors = func(errs []error) error {
		return PlanMultiError{MultiError{errs}}
	}

	// We do a special treatment for -detailed-exitcode since we do not want to interrupt the processing of dependant
	// stacks if one dependency has changes
	detailedExitCode := util.ListContainsElement(terragruntOptions.TerraformCliArgs, "-detailed-exitcode")
	if detailedExitCode {
		util.RemoveElementFromList(terragruntOptions.TerraformCliArgs, "-detailed-exitcode")
	}

	results := make([]moduleResult, 0, len(stack.Modules))
	err := RunModulesWithHandler(stack.Modules, getResultHandler(detailedExitCode, &results), NormalOrder)
	printSummary(terragruntOptions, results)

	// If there is no error, but -detail-exitcode is specified, we return an error with the number of changes.
	if err == nil && detailedExitCode {
		sum := 0
		for _, status := range results {
			sum += status.NbChanges
		}
		err = countError{sum}
	}

	return err
}

// Returns the handler that will be executed after each completion of `terraform plan`
func getResultHandler(detailedExitCode bool, results *[]moduleResult) ModuleHandler {
	return func(module TerraformModule, output string, err error) error {
		warnAboutMissingDependencies(module, output)
		if exitCode, convErr := shell.GetExitCode(err); convErr == nil && detailedExitCode && exitCode == CHANGE_EXIT_CODE {
			// We do not want to consider CHANGE_EXIT_CODE as an error and not execute the dependants because there is an "error" in the dependencies.
			// CHANGE_EXIT_CODE is not an error in this case, it is simply a status. We will reintroduce the exit code at the very end to mimic the behaviour
			// of the native terrafrom plan -detailed-exitcode to exit with CHANGE_EXIT_CODE if there are changes in any of the module in the stack.
			err = nil
		}

		message, count := extractSummaryResultFromPlan(output)

		// We add the result to the result list (there is no concurrency problem because it is handled by the running_module)
		*results = append(*results, moduleResult{module, err, message, count})

		return err
	}
}

// Print a little summary of the plan execution
func printSummary(terragruntOptions *options.TerragruntOptions, results []moduleResult) {
	fmt.Fprintf(terragruntOptions.Writer, "%s\nSummary:\n", separator)
	for _, result := range results {
		errMsg := ""
		if result.Err != nil {
			errMsg = fmt.Sprintf(", Error: %v", result.Err)
		}

		fmt.Fprintf(terragruntOptions.Writer, "    %v : %v%v\n", result.Module.Path, result.Message, errMsg)
	}
}

// Check the output message
func warnAboutMissingDependencies(module TerraformModule, output string) {
	if strings.Contains(output, ": Resource 'data.terraform_remote_state.") {
		var dependenciesMsg string
		if len(module.Dependencies) > 0 {
			dependenciesMsg = fmt.Sprintf(" contains dependencies to %v and", module.Config.Dependencies.Paths)
		}
		module.TerragruntOptions.Logger.Printf("%v%v refers to remote state "+
			"you may have to apply your changes in the dependencies prior running terragrunt plan-all.\n",
			module.Path,
			dependenciesMsg,
		)
	}
}

// Parse the output message to extract a summary
func extractSummaryResultFromPlan(output string) (string, int) {
	const noChange = "No changes. Infrastructure is up-to-date."
	if strings.Contains(output, noChange) {
		return "No change", 0
	}

	result := planResultRegex.FindStringSubmatch(output)
	if len(result) == 0 {
		return "Unable to determine the plan status", -1
	}

	// Count the total number of changes
	sum := 0
	for _, value := range result[1:] {
		count, _ := strconv.Atoi(value)
		sum += count
	}
	if sum != 0 {
		return result[0], sum
	}

	// Sometimes, terraform returns 0 add, 0 change and 0 destroy. We return a more explicit message
	return "No effective change", 0
}

// This is used to return the total number of changes if -detail-exitcode is specified and return an exit code of 2
// to be compliant with the terraform specification.
type countError struct{ count int }

func (err countError) Error() string {
	article, plural := "is", ""
	if err.count > 1 {
		article, plural = "are", "s"
	}
	return fmt.Sprintf("There %s %v change%s to apply", article, err.count, plural)
}

// If there are changes, the exit status must be = 2
func (err countError) ExitStatus() (int, error) {
	if err.count > 0 {
		return 2, nil
	}
	return 0, nil
}

// This is a specialized version of MultiError type
// It handles the exit code differently from the base implementation
type PlanMultiError struct {
	MultiError
}

func (this PlanMultiError) ExitStatus() (int, error) {
	exitCode := NORMAL_EXIT_CODE
	for i := range this.Errors {
		if code, err := shell.GetExitCode(this.Errors[i]); err != nil {
			return UNDEFINED_EXIT_CODE, this
		} else if code == ERROR_EXIT_CODE || code == CHANGE_EXIT_CODE && exitCode == NORMAL_EXIT_CODE {
			// The exit code 1 is more significant that the exit code 2 because it represents an error
			// while 2 represent a warning.
			return UNDEFINED_EXIT_CODE, this
		}
	}
	return exitCode, nil
}
