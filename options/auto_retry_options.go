package options

import (
	"regexp"
	"time"
)

const DefaultRetryMaxAttempts = 3
const DefaultRetrySleepInterval = 5 * time.Second

// DefaultRetryableErrors is a list of errors that are considered transient and
// should be retried.
//
// It's a list of recurring transient errors encountered when calling terraform
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
}

// defaultErrorsConfig builds a default ErrorsConfig using DefaultRetryableErrors
// and default retry timings. Intended as a fallback when no errors{retry} blocks
// are defined in configuration.
func defaultErrorsConfig() *ErrorsConfig {
	compiled := make([]*ErrorsPattern, 0, len(DefaultRetryableErrors))

	for _, pat := range DefaultRetryableErrors {
		re, err := regexp.Compile(pat)
		if err != nil {
			// Should not happen, as patterns are hardcoded and tested
			panic(err)
		}

		compiled = append(compiled, &ErrorsPattern{Pattern: re})
	}

	cfg := &ErrorsConfig{
		Retry:  map[string]*RetryConfig{},
		Ignore: map[string]*IgnoreConfig{},
	}

	if len(compiled) == 0 {
		return cfg
	}

	cfg.Retry["default"] = &RetryConfig{
		Name:             "default",
		RetryableErrors:  compiled,
		MaxAttempts:      DefaultRetryMaxAttempts,
		SleepIntervalSec: int(DefaultRetrySleepInterval / time.Second),
	}

	return cfg
}
