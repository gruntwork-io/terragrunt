package shell

import (
	"regexp"
	"strings"

	"github.com/gruntwork-io/gruntwork-cli/collections"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/go-multierror"
)

// terraformErrorsMatcher List of errors that we know how to explain to the user. The key is a regex that matches the error message, and the value is the explanation.
var terraformErrorsMatcher = map[string]string{
	"(?s).*Error refreshing state: AccessDenied: Access Denied(?s).*":                     "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
	"(?s).*AllAccessDisabled: All access to this object has been disabled(?s).*":          "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
	"(?s).*operation error S3: ListObjectsV2, https response error StatusCode: 301(?s).*": "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
	"(?s).*AuthorizationHeaderMalformed: The authorization header is malformed(?s).*":     "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
	"(?s).*Error: Initialization required(?s).*":                                          "You need to run terragrunt (run-all) init to initialize working directory.",
	"(?s).*Module source has changed(?s).*":                                               "You need to run terragrunt (run-all) init install all required modules.",
	"(?s).*Error finding AWS credentials(?s).*":                                           "Missing AWS credentials. Provide credentials to proceed.",
	"(?s).*Error: No valid credential sources found(?s).*":                                "Missing AWS credentials. Provide credentials to proceed.",
	"(?s).*Error: validating provider credentials(?s).*":                                  "Missing AWS credentials. Provide credentials to proceed.",
	"(?s).*NoCredentialProviders(?s).*":                                                   "Missing AWS credentials. Provide credentials to proceed.",
	"(?s).*client: no valid credential sources(?s).*":                                     "Missing AWS credentials. Provide credentials to proceed.",
}

// ExplainError will try to explain the error to the user, if we know how to do so.
func ExplainError(err error) string {
	multiErrors, ok := err.(*multierror.Error)
	if !ok {
		return ""
	}
	explanations := map[string]string{}

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
				// collect matched explanations
				explanations[explanation] = "1"
			}
		}
	}
	return strings.Join(collections.Keys(explanations), "\n")
}
