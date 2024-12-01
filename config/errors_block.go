package config

import (
	"fmt"
	"regexp"

	"github.com/zclconf/go-cty/cty"
)

// ErrorsConfig represents the top-level errors configuration
type ErrorsConfig struct {
	Retry  []*RetryBlock  `cty:"retry"  hcl:"retry,block"`
	Ignore []*IgnoreBlock `cty:"ignore"  hcl:"ignore,block"`
}

// RetryBlock represents a labeled retry block
type RetryBlock struct {
	Label            string   `cty:"name" hcl:"name,label"`
	RetryableErrors  []string `cty:"retryable_errors" hcl:"retryable_errors"`
	MaxAttempts      int      `cty:"max_attempts" hcl:"max_attempts"`
	SleepIntervalSec int      `cty:"sleep_interval_sec" hcl:"sleep_interval_sec"`
}

// IgnoreBlock represents a labeled ignore block
type IgnoreBlock struct {
	Label           string               `cty:"name" hcl:"name,label"`
	IgnorableErrors []string             `cty:"ignorable_errors" hcl:"ignorable_errors"`
	Message         string               `cty:"message" hcl:"message,optional"`
	Signals         map[string]cty.Value `cty:"signals" hcl:"signals,optional"`
}

// Clone creates a deep copy of ErrorsConfig
func (c *ErrorsConfig) Clone() *ErrorsConfig {
	if c == nil {
		return nil
	}

	clone := &ErrorsConfig{
		Retry:  make([]*RetryBlock, len(c.Retry)),
		Ignore: make([]*IgnoreBlock, len(c.Ignore)),
	}

	// Clone Retry blocks
	for i, retry := range c.Retry {
		clone.Retry[i] = retry.Clone()
	}

	// Clone Ignore blocks
	for i, ignore := range c.Ignore {
		clone.Ignore[i] = ignore.Clone()
	}

	return clone
}

// Merge combines the current ErrorsConfig with another one, with the other config taking precedence
func (c *ErrorsConfig) Merge(other *ErrorsConfig) {
	if other == nil {
		return
	}
	if c == nil {
		*c = *other
		return
	}

	retryMap := make(map[string]*RetryBlock)
	for _, block := range c.Retry {
		retryMap[block.Label] = block
	}

	ignoreMap := make(map[string]*IgnoreBlock)
	for _, block := range c.Ignore {
		ignoreMap[block.Label] = block
	}

	// Merge retry blocks
	for _, otherBlock := range other.Retry {
		if existing, exists := retryMap[otherBlock.Label]; exists {
			existing.RetryableErrors = mergeStringSlices(existing.RetryableErrors, otherBlock.RetryableErrors)
			if otherBlock.MaxAttempts > 0 {
				existing.MaxAttempts = otherBlock.MaxAttempts
			}
			if otherBlock.SleepIntervalSec > 0 {
				existing.SleepIntervalSec = otherBlock.SleepIntervalSec
			}
		} else {
			// Add new block
			retryMap[otherBlock.Label] = otherBlock
		}
	}

	// Merge ignore blocks
	for _, otherBlock := range other.Ignore {
		if existing, exists := ignoreMap[otherBlock.Label]; exists {
			existing.IgnorableErrors = mergeStringSlices(existing.IgnorableErrors, otherBlock.IgnorableErrors)
			if otherBlock.Message != "" {
				existing.Message = otherBlock.Message
			}
			if otherBlock.Signals != nil {
				if existing.Signals == nil {
					existing.Signals = make(map[string]cty.Value)
				}
				for k, v := range otherBlock.Signals {
					existing.Signals[k] = v
				}
			}
		} else {
			// Add new block
			ignoreMap[otherBlock.Label] = otherBlock
		}
	}

	// Convert maps back to slices
	c.Retry = make([]*RetryBlock, 0, len(retryMap))
	for _, block := range retryMap {
		c.Retry = append(c.Retry, block)
	}

	c.Ignore = make([]*IgnoreBlock, 0, len(ignoreMap))
	for _, block := range ignoreMap {
		c.Ignore = append(c.Ignore, block)
	}
}

// mergeStringSlices combines two string slices removing duplicates
func mergeStringSlices(a, b []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(a)+len(b))

	// Add all strings from both slices, skipping duplicates
	for _, s := range append(a, b...) {
		if _, exists := seen[s]; !exists {
			seen[s] = struct{}{}
			result = append(result, s)
		}
	}
	return result
}

// Clone creates a deep copy of RetryBlock
func (r *RetryBlock) Clone() *RetryBlock {
	if r == nil {
		return nil
	}

	clone := &RetryBlock{
		Label:            r.Label,
		MaxAttempts:      r.MaxAttempts,
		SleepIntervalSec: r.SleepIntervalSec,
	}

	// Deep copy RetryableErrors slice
	if r.RetryableErrors != nil {
		clone.RetryableErrors = make([]string, len(r.RetryableErrors))
		copy(clone.RetryableErrors, r.RetryableErrors)
	}

	return clone
}

// Clone creates a deep copy of IgnoreBlock
func (i *IgnoreBlock) Clone() *IgnoreBlock {
	if i == nil {
		return nil
	}

	clone := &IgnoreBlock{
		Label:   i.Label,
		Message: i.Message,
	}

	// Deep copy IgnorableErrors slice
	if i.IgnorableErrors != nil {
		clone.IgnorableErrors = make([]string, len(i.IgnorableErrors))
		copy(clone.IgnorableErrors, i.IgnorableErrors)
	}

	// Deep copy Signals map
	if i.Signals != nil {
		clone.Signals = make(map[string]cty.Value, len(i.Signals))
		for k, v := range i.Signals {
			clone.Signals[k] = v
		}
	}

	return clone
}

// ErrorAction represents the action to take when an error occurs
type ErrorAction struct {
	ShouldIgnore   bool
	ShouldRetry    bool
	IgnoreMessage  string
	IgnoreSignals  map[string]interface{}
	RetryAttempts  int
	RetrySleepSecs int
}

// ProcessError evaluates an error against the configuration and returns the appropriate action
func (c *ErrorsConfig) ProcessError(err error, currentAttempt int) (*ErrorAction, error) {
	if err == nil {
		return nil, nil
	}

	errStr := err.Error()
	action := &ErrorAction{}

	// First check ignore rules
	for _, ignoreBlock := range c.Ignore {
		isIgnorable, err := matchesAnyPattern(errStr, ignoreBlock.IgnorableErrors)
		if err != nil {
			return nil, fmt.Errorf("error processing ignore patterns: %w", err)
		}

		if isIgnorable {
			action.ShouldIgnore = true
			action.IgnoreMessage = ignoreBlock.Message
			action.IgnoreSignals = make(map[string]interface{})

			// Convert cty.Value map to regular map
			for k, v := range ignoreBlock.Signals {
				action.IgnoreSignals[k] = v
			}
			return action, nil
		}
	}

	// Then check retry rules
	for _, retryBlock := range c.Retry {
		isRetryable, err := matchesAnyPattern(errStr, retryBlock.RetryableErrors)
		if err != nil {
			return nil, fmt.Errorf("error processing retry patterns: %w", err)
		}

		if isRetryable {
			if currentAttempt >= retryBlock.MaxAttempts {
				return nil, fmt.Errorf("max retry attempts (%d) reached for error: %v",
					retryBlock.MaxAttempts, err)
			}

			action.ShouldRetry = true
			action.RetryAttempts = retryBlock.MaxAttempts
			action.RetrySleepSecs = retryBlock.SleepIntervalSec
			return action, nil
		}
	}

	// If no rules match, return the original error
	return nil, err
}

// matchesAnyPattern checks if the input string matches any of the provided patterns
func matchesAnyPattern(input string, patterns []string) (bool, error) {
	for _, pattern := range patterns {
		// Handle negative patterns (patterns starting with !)
		isNegative := false
		if len(pattern) > 0 && pattern[0] == '!' {
			isNegative = true
			pattern = pattern[1:]
		}

		matched, err := regexp.MatchString(pattern, input)
		if err != nil {
			return false, fmt.Errorf("invalid pattern %q: %w", pattern, err)
		}

		if matched {
			return !isNegative, nil
		}
	}
	return false, nil
}
