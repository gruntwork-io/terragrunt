package options

import (
	"time"

	"github.com/gruntwork-io/terragrunt/internal/errorconfig"
	"github.com/gruntwork-io/terragrunt/internal/retry"
)

// defaultErrorsConfig builds a default errorconfig.Config using retry.DefaultRetryableRegexps
// and default retry timings. Intended as a fallback when no errors{retry} blocks
// are defined in configuration.
func defaultErrorsConfig() *errorconfig.Config {
	compiled := make([]*errorconfig.Pattern, len(retry.DefaultRetryableRegexps))

	for i, re := range retry.DefaultRetryableRegexps {
		compiled[i] = &errorconfig.Pattern{Pattern: re}
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
