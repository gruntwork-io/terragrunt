package options

import (
	"regexp"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/errorconfig"
	"github.com/gruntwork-io/terragrunt/internal/retry"
)

// defaultErrorsConfig builds a default errorconfig.Config using retry.DefaultRetryableErrors
// and default retry timings. Intended as a fallback when no errors{retry} blocks
// are defined in configuration.
func defaultErrorsConfig() *errorconfig.Config {
	compiled := make([]*errorconfig.Pattern, 0, len(retry.DefaultRetryableErrors))

	for _, pat := range retry.DefaultRetryableErrors {
		re, err := regexp.Compile(pat)
		if err != nil {
			// Should not happen, as patterns are hardcoded and tested
			panic(err)
		}

		compiled = append(compiled, &errorconfig.Pattern{Pattern: re})
	}

	cfg := &errorconfig.Config{
		Retry:  map[string]*errorconfig.RetryConfig{},
		Ignore: map[string]*errorconfig.IgnoreConfig{},
	}

	if len(compiled) == 0 {
		return cfg
	}

	cfg.Retry["default"] = &errorconfig.RetryConfig{
		Name:             "default",
		RetryableErrors:  compiled,
		MaxAttempts:      retry.DefaultMaxAttempts,
		SleepIntervalSec: int(retry.DefaultSleepInterval / time.Second),
	}

	return cfg
}
