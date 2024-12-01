package config

import (
	"github.com/gruntwork-io/terragrunt/util"
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
			existing.RetryableErrors = util.MergeStringSlices(existing.RetryableErrors, otherBlock.RetryableErrors)

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
			existing.IgnorableErrors = util.MergeStringSlices(existing.IgnorableErrors, otherBlock.IgnorableErrors)

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
