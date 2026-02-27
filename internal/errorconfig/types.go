// Package errorconfig defines types for structured error handling configuration.
package errorconfig

import (
	"fmt"
	"maps"
	"regexp"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// errorCleanPattern is used to clean error messages when looking for retry and ignore patterns.
var errorCleanPattern = regexp.MustCompile(`[^a-zA-Z0-9./'"():=\- ]+`)

// Config is the extracted errors handling configuration.
type Config struct {
	Retry  map[string]*RetryConfig
	Ignore map[string]*IgnoreConfig
}

// RetryConfig represents the configuration for retrying specific errors.
type RetryConfig struct {
	Name             string
	RetryableErrors  []*Pattern
	MaxAttempts      int
	SleepIntervalSec int
}

// IgnoreConfig represents the configuration for ignoring specific errors.
type IgnoreConfig struct {
	Signals         map[string]any
	Name            string
	Message         string
	IgnorableErrors []*Pattern
}

// Pattern represents a regex pattern for matching errors, with optional negation.
type Pattern struct {
	Pattern  *regexp.Regexp `clone:"shadowcopy"`
	Negative bool
}

// Action represents the action to take when an error occurs.
type Action struct {
	IgnoreSignals   map[string]any
	IgnoreBlockName string
	RetryBlockName  string
	IgnoreMessage   string
	RetryAttempts   int
	RetrySleepSecs  int
	ShouldIgnore    bool
	ShouldRetry     bool
}

// MaxAttemptsReachedError is returned when the maximum number of retry attempts is reached.
type MaxAttemptsReachedError struct {
	Err        error
	MaxRetries int
}

func (e *MaxAttemptsReachedError) Error() string {
	return fmt.Sprintf("max retry attempts (%d) reached for error: %v", e.MaxRetries, e.Err)
}

// AttemptErrorRecovery attempts to recover from an error by checking the ignore and retry rules.
func (c *Config) AttemptErrorRecovery(l log.Logger, err error, currentAttempt int) (*Action, error) {
	if err == nil {
		return nil, nil
	}

	errStr := extractErrorMessage(err)
	action := &Action{}

	l.Debugf("Attempting error recovery for error: %s", errStr)

	// First check ignore rules
	for _, ignoreBlock := range c.Ignore {
		isIgnorable := matchesAnyRegexpPattern(errStr, ignoreBlock.IgnorableErrors)
		if !isIgnorable {
			continue
		}

		action.IgnoreBlockName = ignoreBlock.Name
		action.ShouldIgnore = true
		action.IgnoreMessage = ignoreBlock.Message
		action.IgnoreSignals = make(map[string]any)

		// Convert cty.Value map to regular map
		maps.Copy(action.IgnoreSignals, ignoreBlock.Signals)

		return action, nil
	}

	// Then check retry rules
	for _, retryBlock := range c.Retry {
		isRetryable := matchesAnyRegexpPattern(errStr, retryBlock.RetryableErrors)
		if !isRetryable {
			continue
		}

		if currentAttempt >= retryBlock.MaxAttempts {
			return nil, &MaxAttemptsReachedError{
				MaxRetries: retryBlock.MaxAttempts,
				Err:        err,
			}
		}

		action.RetryBlockName = retryBlock.Name
		action.ShouldRetry = true
		action.RetryAttempts = retryBlock.MaxAttempts
		action.RetrySleepSecs = retryBlock.SleepIntervalSec

		return action, nil
	}

	// We encountered no error while attempting error recovery, even though the underlying error
	// is still present. Recovery did not error, the original error will be handled externally.
	return nil, nil
}

func extractErrorMessage(err error) string {
	var errText string

	// For ProcessExecutionError, match only against stderr and the underlying error,
	// not the full command string with flags.
	var processErr util.ProcessExecutionError
	if errors.As(err, &processErr) {
		errText = processErr.Output.Stderr.String() + "\n" + processErr.Err.Error()
	} else {
		errText = err.Error()
	}

	multilineText := log.RemoveAllASCISeq(errText)
	errorText := errorCleanPattern.ReplaceAllString(multilineText, " ")

	return strings.Join(strings.Fields(errorText), " ")
}

// matchesAnyRegexpPattern checks if the input string matches any of the provided compiled patterns.
func matchesAnyRegexpPattern(input string, patterns []*Pattern) bool {
	for _, pattern := range patterns {
		isNegative := pattern.Negative
		matched := pattern.Pattern.MatchString(input)

		if matched {
			return !isNegative
		}
	}

	return false
}
