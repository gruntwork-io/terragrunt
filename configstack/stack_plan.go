package configstack

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// The returned information for each module
type moduleResult struct {
	Module    TerraformModule
	Err       error
	Message   string
	NbChanges int
}

func (stack *Stack) planWithSummary(terragruntOptions *options.TerragruntOptions) error {
	// We do a special treatment for -detailed-exitcode since we do not want to interrupt the processing of dependant
	// stacks if one dependency has changes
	detailedExitCode := util.ListContainsElement(terragruntOptions.TerraformCliArgs, "-detailed-exitcode")
	if detailedExitCode {
		util.RemoveElementFromList(terragruntOptions.TerraformCliArgs, "-detailed-exitcode")
	}

	results := make([]moduleResult, 0, len(stack.Modules))
	var mutex sync.Mutex
	err := RunModulesWithHandler(stack.Modules, getResultHandler(detailedExitCode, &results, &mutex), NormalOrder)
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

// Returns the handler and the list of results
func getResultHandler(detailedExitCode bool, results *[]moduleResult, mutex *sync.Mutex) ModuleHandler {
	return func(module TerraformModule, output string, err error) error {
		inspectPlanResult(module, output)
		if exitCode, convErr := shell.GetExitCode(err); convErr == nil && detailedExitCode && exitCode == 2 {
			err = nil
		}

		message, count := extractSummaryResultFromPlan(output)

		// We add the result to the result list (using mutex to ensure that there is no conflict while appending to the result array)
		mutex.Lock()
		defer mutex.Unlock()
		*results = append(*results, moduleResult{module, err, message, count})

		return err
	}
}

// Print a little summary of the plan execution
func printSummary(terragruntOptions *options.TerragruntOptions, results []moduleResult) {
	fmt.Fprintln(terragruntOptions.Writer, "Summary:")
	for _, result := range results {
		errMsg := ""
		if result.Err != nil {
			errMsg = fmt.Sprintf(", Error: %v", result.Err)
		}

		fmt.Fprintf(terragruntOptions.Writer, "    %v : %v%v\n", result.Module.Path, result.Message, errMsg)
	}
}

// Check the output message
func inspectPlanResult(module TerraformModule, output string) {
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

	re := regexp.MustCompile(`(\d+) to add, (\d+) to change, (\d+) to destroy.`)
	result := re.FindStringSubmatch(output)
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
	article, s := "is", ""
	if err.count > 1 {
		article, s = "are", "s"
	}
	return fmt.Sprintf("There %s %v change%s to apply", article, err.count, s)
}

// If there are changes, the exit status must be = 2
func (err countError) ExitStatus() (int, error) {
	if err.count > 0 {
		return 2, nil
	}
	return 0, nil
}
