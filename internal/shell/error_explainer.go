package shell

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/util"
)

// terraformErrorsMatcher List of errors that we know how to explain to the user. The key is a regex that matches the error message, and the value is the explanation.
var terraformErrorsMatcher = map[string]string{
	"(?s).*Error refreshing state: AccessDenied: Access Denied(?s).*":                     "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
	"(?s).*AllAccessDisabled: All access to this object has been disabled(?s).*":          "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
	"(?s).*operation error S3: ListObjectsV2, https response error StatusCode: 301(?s).*": "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
	"(?s).*The authorization header is malformed(?s).*":                                   "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
	"(?s).*Unable to list objects in S3 bucket(?s).*":                                     "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
	"(?s).*Error: Initialization required(?s).*":                                          "You need to run terragrunt (run --all) init to initialize working directory.",
	"(?s).*Unit source has changed(?s).*":                                                 "You need to run terragrunt (run --all) init install all required modules.",
	"(?s).*Error finding AWS credentials(?s).*":                                           "Missing AWS credentials. Provide credentials to proceed.",
	"(?s).*Error: No valid credential sources found(?s).*":                                "Missing AWS credentials. Provide credentials to proceed.",
	"(?s).*Error: validating provider credentials(?s).*":                                  "Missing AWS credentials. Provide credentials to proceed.",
	"(?s).*NoCredentialProviders(?s).*":                                                   "Missing AWS credentials. Provide credentials to proceed.",
	"(?s).*client: no valid credential sources(?s).*":                                     "Missing AWS credentials. Provide credentials to proceed.",
	"(?s).*exec: \"(tofu|terraform)\": executable file not found(?s).*":                   "The executables 'terraform' and 'tofu' are missing from your $PATH. Please add at least one of these to your $PATH.",
	"(?s).*bucket must have been previously created.*":                                    "Remote state bucket not found, create it manually or rerun with --backend-bootstrap to provision automatically.",
	"(?s).*specified bucket does not exist.*":                                             "Remote state bucket not found, create it manually or rerun with --backend-bootstrap to provision automatically.",
	"(?s).*S3 bucket does not exist.*":                                                    "Remote state bucket not found, create it manually or rerun with --backend-bootstrap to provision automatically.",
}

// ExplainError will try to explain the error to the user, if we know how to do so.
func ExplainError(err error) string {
	explanations := map[string]string{}

	for _, err := range flattenErrorChain(err) {
		message := err.Error()

		// extract process output, if it is the case
		var processError util.ProcessExecutionError
		if ok := errors.As(err, &processError); ok {
			errorOutput := processError.Output.Stderr.String()
			stdOut := processError.Output.Stdout.String()
			message = fmt.Sprintf("%s\n%s", stdOut, errorOutput)
		}

		for regex, explanation := range terraformErrorsMatcher {
			if match, _ := regexp.MatchString(regex, message); match {
				explanations[explanation] = "1"
			}
		}
	}

	return strings.Join(slices.Sorted(maps.Keys(explanations)), "\n")
}

// flattenErrorChain walks both single-error (Unwrap() error) and joined-error
// (Unwrap() []error) chains, returning every error encountered including the root.
func flattenErrorChain(err error) []error {
	var (
		out  []error
		walk func(error)
	)

	walk = func(e error) {
		for e != nil {
			out = append(out, e)
			if multi, ok := e.(interface{ Unwrap() []error }); ok {
				for _, child := range multi.Unwrap() {
					walk(child)
				}

				return
			}

			e = errors.Unwrap(e)
		}
	}

	walk(err)

	return out
}
