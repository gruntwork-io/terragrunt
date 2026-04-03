// Package retry provides default retry configuration for Terragrunt.
package retry

import (
	"fmt"
	"regexp"
	"time"
)

// DefaultMaxAttempts is the default number of retry attempts.
const DefaultMaxAttempts = 3

// DefaultSleepInterval is the default sleep interval between retries.
const DefaultSleepInterval = 5 * time.Second

// DefaultRetryableErrors is a list of errors that are considered transient and
// should be retried.
//
// It's a list of recurring transient errors encountered when calling terraform.
// If any of these match, we'll retry the command.
var DefaultRetryableErrors = []string{
	"(?s).*Failed to load state.*tcp.*timeout.*",
	"(?s).*Failed to load backend.*TLS handshake timeout.*",
	"(?s).*Creating metric alarm failed.*request to update this alarm is in progress.*",
	"(?s).*Error installing provider.*TLS handshake timeout.*",
	"(?s).*Error configuring the backend.*TLS handshake timeout.*",
	"(?s).*Error installing provider.*tcp.*timeout.*",
	"(?s).*Error installing provider.*tcp.*connection reset by peer.*",
	"NoSuchBucket: The specified bucket does not exist",
	"(?s).*Error creating SSM parameter: TooManyUpdates:.*",
	"(?s).*app.terraform.io.*: 429 Too Many Requests.*",
	"(?s).*ssh_exchange_identification.*Connection closed by remote host.*",
	"(?s).*Client\\.Timeout exceeded while awaiting headers.*",
	"(?s).*Could not download module.*The requested URL returned error: 429.*",
	"(?s).*net/http: TLS.*handshake timeout.*",
	"(?s).*could not query provider registry.*context deadline exceeded.*",
	"(?s).*provider.*TLS handshake timeout.*",
	"(?s).*provider.*tcp.*timeout.*",
	"(?s).*provider.*tcp.*connection reset by peer.*",
	"(?s).*provider.*context deadline exceeded.*",
	"(?s).*registry.*context deadline exceeded.*",
	"(?s).*Failed to resolve provider.*timeout.*",
	"(?s).*Failed to resolve provider.*connection reset by peer.*",
	"(?s).*Failed to resolve provider.*context deadline exceeded.*",
	"(?s).*could not connect to registry.*timeout.*",
	"(?s).*could not connect to registry.*connection reset by peer.*",
	"(?s).*could not connect to registry.*context deadline exceeded.*",
	"(?s).*failed to request discovery document.*context deadline exceeded.*",
	"(?s).*Failed to query available provider packages.*timeout.*",
	"(?s).*Failed to query available provider packages.*connection reset by peer.*",
	"(?s).*Failed to query available provider packages.*context deadline exceeded.*",
}

// DefaultRetryableRegexps contains pre-compiled regexps for DefaultRetryableErrors,
// populated at init time. Do not modify.
var DefaultRetryableRegexps []*regexp.Regexp

func init() {
	DefaultRetryableRegexps = make([]*regexp.Regexp, len(DefaultRetryableErrors))
	for i, pat := range DefaultRetryableErrors {
		re, err := regexp.Compile(pat)
		if err != nil {
			panic(fmt.Sprintf("retry: pattern %d failed to compile: %q: %v", i, pat, err))
		}

		DefaultRetryableRegexps[i] = re
	}
}
