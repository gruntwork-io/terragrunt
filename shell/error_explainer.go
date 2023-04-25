package shell

import (
	"fmt"
	"regexp"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/hashicorp/go-multierror"
)

// terraformErrorsMatcher List of errors that we know how to explain to the user. The key is a regex that matches the error message, and the value is the explanation.
var terraformErrorsMatcher = map[string]string{
	"(?s).*Error refreshing state: AccessDenied: Access Denied(?s).*": "You don't have access to the S3 bucket where the state is stored. Check your AWS credentials and permissions.",
	"(?s).*Error: Initialization required(?s).*":                      "You need to run terragrunt (run-all) init to initialize working directory.",
	"(?s).*Module source has changed(?s).*":                           "You need to run terragrunt (run-all) init install all required modules.",
}

// ExplainError will try to explain the error to the user, if we know how to do so.
func ExplainError(err error) string {
	var result string
	multiErrors, ok := err.(*multierror.Error)
	if !ok {
		return result
	}

	// iterate over each error, unwrap it, and check for error output
	for _, errorItem := range multiErrors.Errors {
		originalError := errors.Unwrap(errorItem)
		if originalError == nil {
			continue
		}
		processError, ok := originalError.(ProcessExecutionError)
		if !ok {
			continue
		}
		errorOutput := processError.Stderr
		for regex, explanation := range terraformErrorsMatcher {
			if match, _ := regexp.MatchString(regex, errorOutput); match {
				// append explanation to result
				result = fmt.Sprintf("%s\n%s", result, explanation)
			}
		}
	}
	return result
}
