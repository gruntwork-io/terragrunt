// Package retry provides default retry configuration for Terragrunt.
package retry

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// DefaultMaxAttempts is the default number of retry attempts.
const DefaultMaxAttempts = 3

// DefaultSleepInterval is the default sleep interval between retries.
const DefaultSleepInterval = 5 * time.Second

// DefaultRetryableRegexps contains pre-compiled regexps for transient errors
// encountered when calling terraform/tofu. If any match, the command is retried.
var DefaultRetryableRegexps = func() []*regexp.Regexp {
	patterns := []string{
		`(?s).*Failed to load state.*tcp.*timeout.*`,
		`(?s).*Failed to load backend.*TLS handshake timeout.*`,
		`(?s).*Creating metric alarm failed.*request to update this alarm is in progress.*`,
		`(?s).*Error installing provider.*TLS handshake timeout.*`,
		`(?s).*Error configuring the backend.*TLS handshake timeout.*`,
		`(?s).*Error installing provider.*tcp.*timeout.*`,
		`(?s).*Error installing provider.*tcp.*connection reset by peer.*`,
		`NoSuchBucket: The specified bucket does not exist`,
		`(?s).*Error creating SSM parameter: TooManyUpdates:.*`,
		`(?s).*app.terraform.io.*: 429 Too Many Requests.*`,
		`(?s).*ssh_exchange_identification.*Connection closed by remote host.*`,
		`(?s).*Client\.Timeout exceeded while awaiting headers.*`,
		`(?s).*Could not download module.*The requested URL returned error: 429.*`,
		`(?s).*net/http: TLS.*handshake timeout.*`,
		`(?s).*could not query provider registry.*context deadline exceeded.*`,
		`(?s).*provider.*TLS handshake timeout.*`,
		`(?s).*provider.*tcp.*timeout.*`,
		`(?s).*provider.*tcp.*connection reset by peer.*`,
		`(?s).*provider.*context deadline exceeded.*`,
		`(?s).*registry.*context deadline exceeded.*`,
		`(?s).*Failed to resolve provider.*timeout.*`,
		`(?s).*Failed to resolve provider.*connection reset by peer.*`,
		`(?s).*Failed to resolve provider.*context deadline exceeded.*`,
		`(?s).*could not connect to registry.*timeout.*`,
		`(?s).*could not connect to registry.*connection reset by peer.*`,
		`(?s).*could not connect to registry.*context deadline exceeded.*`,
		`(?s).*failed to request discovery document.*context deadline exceeded.*`,
		`(?s).*Failed to query available provider packages.*timeout.*`,
		`(?s).*Failed to query available provider packages.*connection reset by peer.*`,
		`(?s).*Failed to query available provider packages.*context deadline exceeded.*`,
	}

	var errs []string

	compiled := make([]*regexp.Regexp, len(patterns))
	for i, pat := range patterns {
		re, err := regexp.Compile(pat)
		if err != nil {
			errs = append(errs, fmt.Sprintf("  pattern %d: %q: %v", i, pat, err))
			continue
		}

		compiled[i] = re
	}

	if len(errs) > 0 {
		panic(fmt.Sprintf("retry: %d default retryable error pattern(s) failed to compile:\n%s",
			len(errs), strings.Join(errs, "\n")))
	}

	return compiled
}()

// DefaultRetryableErrors contains the string form of each pattern in
// DefaultRetryableRegexps, for exposing patterns to HCL functions.
var DefaultRetryableErrors = func() []string {
	strs := make([]string, len(DefaultRetryableRegexps))
	for i, re := range DefaultRetryableRegexps {
		strs[i] = re.String()
	}

	return strs
}()
