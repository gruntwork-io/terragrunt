package shell

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/util"
)

const (
	s3AccessDeniedExplanation        = "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions."
	missingAWSCredentialsExplanation = "Missing AWS credentials. Provide credentials to proceed."
)

// terraformErrorsMatcher returns the errors we know how to explain to the user. Each key matches an error message,
// and its value is the explanation. The patterns are compiled on first use, so a run that never fails never pays for
// them.
var terraformErrorsMatcher = sync.OnceValue(func() map[*regexp.Regexp]string {
	return map[*regexp.Regexp]string{
		regexp.MustCompile("(?s).*Error refreshing state: AccessDenied: Access Denied(?s).*"):                     s3AccessDeniedExplanation,
		regexp.MustCompile("(?s).*AllAccessDisabled: All access to this object has been disabled(?s).*"):          s3AccessDeniedExplanation,
		regexp.MustCompile("(?s).*operation error S3: ListObjectsV2, https response error StatusCode: 301(?s).*"): s3AccessDeniedExplanation,
		regexp.MustCompile("(?s).*The authorization header is malformed(?s).*"):                                   s3AccessDeniedExplanation,
		regexp.MustCompile("(?s).*Unable to list objects in S3 bucket(?s).*"):                                     s3AccessDeniedExplanation,
		regexp.MustCompile("(?s).*Error: Initialization required(?s).*"):                                          "You need to run terragrunt (run --all) init to initialize working directory.",
		regexp.MustCompile("(?s).*Unit source has changed(?s).*"):                                                 "You need to run terragrunt (run --all) init install all required modules.",
		regexp.MustCompile("(?s).*Error finding AWS credentials(?s).*"):                                           missingAWSCredentialsExplanation,
		regexp.MustCompile("(?s).*Error: No valid credential sources found(?s).*"):                                missingAWSCredentialsExplanation,
		regexp.MustCompile("(?s).*Error: validating provider credentials(?s).*"):                                  missingAWSCredentialsExplanation,
		regexp.MustCompile("(?s).*NoCredentialProviders(?s).*"):                                                   missingAWSCredentialsExplanation,
		regexp.MustCompile("(?s).*client: no valid credential sources(?s).*"):                                     missingAWSCredentialsExplanation,
		regexp.MustCompile("(?s).*exec: \"(tofu|terraform)\": executable file not found(?s).*"):                   "The executables 'terraform' and 'tofu' are missing from your $PATH. Please add at least one of these to your $PATH.",
		regexp.MustCompile("(?s).*bucket must have been previously created.*"):                                    "Remote state bucket not found, create it manually or rerun with --backend-bootstrap to provision automatically.",
		regexp.MustCompile("(?s).*specified bucket does not exist.*"):                                             "Remote state bucket not found, create it manually or rerun with --backend-bootstrap to provision automatically.",
		regexp.MustCompile("(?s).*S3 bucket does not exist.*"):                                                    "Remote state bucket not found, create it manually or rerun with --backend-bootstrap to provision automatically.",
	}
})

// ExplainError will try to explain the error to the user, if we know how to do so.
func ExplainError(err error) string {
	explanations := map[string]string{}

	for _, err := range flattenErrorChain(err) {
		message := err.Error()

		// extract process output, if it is the case
		var processError *util.ProcessExecutionError
		if ok := errors.As(err, &processError); ok {
			errorOutput := processError.Output.Stderr.String()
			stdOut := processError.Output.Stdout.String()
			message = fmt.Sprintf("%s\n%s", stdOut, errorOutput)
		}

		for regex, explanation := range terraformErrorsMatcher() {
			if regex.MatchString(message) {
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
