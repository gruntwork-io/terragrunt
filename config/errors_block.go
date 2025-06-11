package config

import (
	"maps"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/zclconf/go-cty/cty"
)

// ErrorsConfig represents the top-level errors configuration
type ErrorsConfig struct {
	Retry  []*RetryBlock  `cty:"retry" hcl:"retry,block"`
	Ignore []*IgnoreBlock `cty:"ignore" hcl:"ignore,block"`
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
	Signals         map[string]cty.Value `cty:"signals" hcl:"signals,optional"`
	Label           string               `cty:"name" hcl:"name,label"`
	Message         string               `cty:"message" hcl:"message,optional"`
	IgnorableErrors []string             `cty:"ignorable_errors" hcl:"ignorable_errors"`
}

// Clone returns a deep copy of ErrorsConfig
func (c *ErrorsConfig) Clone() *ErrorsConfig {
	if c == nil {
		return nil
	}

	return &ErrorsConfig{
		Retry:  cloneRetryBlocks(c.Retry),
		Ignore: cloneIgnoreBlocks(c.Ignore),
	}
}

// Merge combines the current ErrorsConfig with another one, prioritizing the other config
func (c *ErrorsConfig) Merge(other *ErrorsConfig) {
	if c == nil || other == nil {
		return
	}

	c.Retry = mergeRetryBlocks(c.Retry, other.Retry)
	c.Ignore = mergeIgnoreBlocks(c.Ignore, other.Ignore)
}

// Clone returns a deep copy of a RetryBlock
func (r *RetryBlock) Clone() *RetryBlock {
	if r == nil {
		return nil
	}

	return &RetryBlock{
		Label:            r.Label,
		RetryableErrors:  cloneStringSlice(r.RetryableErrors),
		MaxAttempts:      r.MaxAttempts,
		SleepIntervalSec: r.SleepIntervalSec,
	}
}

// Clone returns a deep copy of an IgnoreBlock
func (i *IgnoreBlock) Clone() *IgnoreBlock {
	if i == nil {
		return nil
	}

	return &IgnoreBlock{
		Label:           i.Label,
		IgnorableErrors: cloneStringSlice(i.IgnorableErrors),
		Message:         i.Message,
		Signals:         cloneSignalsMap(i.Signals),
	}
}

// Helper function to deep copy a slice of RetryBlock
func cloneRetryBlocks(blocks []*RetryBlock) []*RetryBlock {
	if blocks == nil {
		return nil
	}

	cloned := make([]*RetryBlock, len(blocks))
	for i, block := range blocks {
		cloned[i] = block.Clone()
	}

	return cloned
}

// Helper function to deep copy a slice of IgnoreBlock
func cloneIgnoreBlocks(blocks []*IgnoreBlock) []*IgnoreBlock {
	if blocks == nil {
		return nil
	}

	cloned := make([]*IgnoreBlock, len(blocks))
	for i, block := range blocks {
		cloned[i] = block.Clone()
	}

	return cloned
}

// Helper function to deep copy a slice of strings
func cloneStringSlice(slice []string) []string {
	if slice == nil {
		return nil
	}

	cloned := make([]string, len(slice))
	copy(cloned, slice)

	return cloned
}

// Helper function to deep copy a map of signals
func cloneSignalsMap(signals map[string]cty.Value) map[string]cty.Value {
	if signals == nil {
		return nil
	}

	cloned := make(map[string]cty.Value, len(signals))
	maps.Copy(cloned, signals)

	return cloned
}

// Merges two slices of RetryBlock, prioritizing the second slice
func mergeRetryBlocks(existing, other []*RetryBlock) []*RetryBlock {
	retryMap := make(map[string]*RetryBlock, len(existing)+len(other))

	// Add existing retry blocks
	for _, block := range existing {
		retryMap[block.Label] = block
	}

	// Merge retry blocks from 'other'
	for _, otherBlock := range other {
		if existingBlock, found := retryMap[otherBlock.Label]; found {
			existingBlock.RetryableErrors = util.MergeStringSlices(existingBlock.RetryableErrors, otherBlock.RetryableErrors)

			if otherBlock.MaxAttempts > 0 {
				existingBlock.MaxAttempts = otherBlock.MaxAttempts
			}

			if otherBlock.SleepIntervalSec > 0 {
				existingBlock.SleepIntervalSec = otherBlock.SleepIntervalSec
			}

			continue
		}

		retryMap[otherBlock.Label] = otherBlock
	}

	return util.MapToSlice(retryMap)
}

// Merges two slices of IgnoreBlock, prioritizing the second slice
func mergeIgnoreBlocks(existing, other []*IgnoreBlock) []*IgnoreBlock {
	ignoreMap := make(map[string]*IgnoreBlock, len(existing)+len(other))

	// Add existing ignore blocks
	for _, block := range existing {
		ignoreMap[block.Label] = block
	}

	// Merge ignore blocks from 'other'
	for _, otherBlock := range other {
		if existingBlock, found := ignoreMap[otherBlock.Label]; found {
			existingBlock.IgnorableErrors = util.MergeStringSlices(existingBlock.IgnorableErrors, otherBlock.IgnorableErrors)

			if otherBlock.Message != "" {
				existingBlock.Message = otherBlock.Message
			}

			if otherBlock.Signals != nil {
				if existingBlock.Signals == nil {
					existingBlock.Signals = make(map[string]cty.Value, len(otherBlock.Signals))
				}

				maps.Copy(existingBlock.Signals, otherBlock.Signals)
			}
		} else {
			ignoreMap[otherBlock.Label] = otherBlock
		}
	}

	// Convert map back to slice
	return util.MapToSlice(ignoreMap)
}
