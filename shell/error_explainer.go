package shell

import (
	goErrors "errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/gruntwork-cli/collections"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/go-multierror"
)

// terraformErrorsMatcher List of errors that we know how to explain to the user. The key is a regex that matches the error message, and the value is the explanation.
var terraformErrorsMatcher = map[string]string{
	"(?s).*Error refreshing state: AccessDenied: Access Denied(?s).*":                     "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
	"(?s).*AllAccessDisabled: All access to this object has been disabled(?s).*":          "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
	"(?s).*operation error S3: ListObjectsV2, https response error StatusCode: 301(?s).*": "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
	"(?s).*The authorization header is malformed(?s).*":                                   "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
	"(?s).*Unable to list objects in S3 bucket(?s).*":                                     "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
	"(?s).*Error: Initialization required(?s).*":                                          "You need to run terragrunt (run-all) init to initialize working directory.",
	"(?s).*Module source has changed(?s).*":                                               "You need to run terragrunt (run-all) init install all required modules.",
	"(?s).*Error finding AWS credentials(?s).*":                                           "Missing AWS credentials. Provide credentials to proceed.",
	"(?s).*Error: No valid credential sources found(?s).*":                                "Missing AWS credentials. Provide credentials to proceed.",
	"(?s).*Error: validating provider credentials(?s).*":                                  "Missing AWS credentials. Provide credentials to proceed.",
	"(?s).*NoCredentialProviders(?s).*":                                                   "Missing AWS credentials. Provide credentials to proceed.",
	"(?s).*client: no valid credential sources(?s).*":                                     "Missing AWS credentials. Provide credentials to proceed.",
	"(?s).*exec: \"(tofu|terraform)\": executable file not found(?s).*":                   "The executables 'terraform' and 'tofu' are missing from your $PATH. Please add at least one of these to your $PATH.",
}

// ExplainError will try to explain the error to the user, if we know how to do so.
func ExplainError(err error) string {
	errorsToProcess := []error{err}
	var multiErrors *multierror.Error
	// multiErrors, ok := err.(*multierror.Error)
	if ok := goErrors.As(err, &multiErrors); ok {
		errorsToProcess = multiErrors.Errors
	}
	explanations := map[string]string{}

	// iterate over each error, unwrap it, and check for error output
	for _, errorItem := range errorsToProcess {
		originalError := errors.Unwrap(errorItem)
		if originalError == nil {
			continue
		}
		message := originalError.Error()
		// extract process output, if it is the case
		var processError util.ProcessExecutionError
		if ok := goErrors.As(originalError, &processError); ok {
			errorOutput := processError.Stderr
			stdOut := processError.StdOut
			message = fmt.Sprintf("%s\n%s", stdOut, errorOutput)
		}
		for regex, explanation := range terraformErrorsMatcher {
			if match, _ := regexp.MatchString(regex, message); match {
				// collect matched explanations
				explanations[explanation] = "1"
			}
		}
	}
	return strings.Join(collections.Keys(explanations), "\n")
}
