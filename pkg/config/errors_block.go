package config

import (
	"maps"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/zclconf/go-cty/cty"
)

// ErrorsConfig represents the top-level errors configuration
type ErrorsConfig struct {
	Retry  []*RetryBlock  `cty:"retry"  hcl:"retry,block"`
	Ignore []*IgnoreBlock `cty:"ignore" hcl:"ignore,block"`
}

// RetryBlock represents a labeled retry block
type RetryBlock struct {
	Label            string   `cty:"name"               hcl:"name,label"`
	RetryableErrors  []string `cty:"retryable_errors"   hcl:"retryable_errors"`
	MaxAttempts      int      `cty:"max_attempts"       hcl:"max_attempts"`
	SleepIntervalSec int      `cty:"sleep_interval_sec" hcl:"sleep_interval_sec"`
}

// IgnoreBlock represents a labeled ignore block
type IgnoreBlock struct {
	Signals         map[string]cty.Value `cty:"signals"          hcl:"signals,optional"`
	Label           string               `cty:"name"             hcl:"name,label"`
	Message         string               `cty:"message"          hcl:"message,optional"`
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

// lastPerLabel returns a copy of c with one retry block and one ignore block for each label: the last block declared
// with the label, at the position of the first. It returns nil when c is nil.
func (c *ErrorsConfig) lastPerLabel() *ErrorsConfig {
	if c == nil {
		return nil
	}

	return &ErrorsConfig{
		Retry:  lastBlockPerLabel(c.Retry),
		Ignore: lastBlockPerLabel(c.Ignore),
	}
}

// label returns the label of the retry block.
func (r *RetryBlock) label() string {
	return r.Label
}

// label returns the label of the ignore block.
func (i *IgnoreBlock) label() string {
	return i.Label
}

// mergeRetryBlocks returns the blocks of existing followed by the blocks of other, each in declaration order. A block
// of other whose label is in existing is not appended: it updates the existing block where that block stands, adding
// its patterns and replacing max_attempts and sleep_interval_sec when it sets them.
func mergeRetryBlocks(existing, other []*RetryBlock) []*RetryBlock {
	merged := slices.Clone(existing)
	indexByLabel := make(map[string]int, len(existing)+len(other))

	for i, block := range merged {
		indexByLabel[block.Label] = i
	}

	for _, otherBlock := range other {
		i, found := indexByLabel[otherBlock.Label]
		if !found {
			merged = append(merged, otherBlock)

			continue
		}

		existingBlock := merged[i]
		existingBlock.RetryableErrors = util.MergeSlices(
			existingBlock.RetryableErrors,
			otherBlock.RetryableErrors,
		)

		if otherBlock.MaxAttempts > 0 {
			existingBlock.MaxAttempts = otherBlock.MaxAttempts
		}

		if otherBlock.SleepIntervalSec > 0 {
			existingBlock.SleepIntervalSec = otherBlock.SleepIntervalSec
		}
	}

	return merged
}

// mergeIgnoreBlocks returns the blocks of existing followed by the blocks of other, each in declaration order. A block
// of other whose label is in existing is not appended: it updates the existing block where that block stands, adding
// its patterns and signals and replacing message when it sets one.
func mergeIgnoreBlocks(existing, other []*IgnoreBlock) []*IgnoreBlock {
	merged := slices.Clone(existing)
	indexByLabel := make(map[string]int, len(existing)+len(other))

	for i, block := range merged {
		indexByLabel[block.Label] = i
	}

	for _, otherBlock := range other {
		i, found := indexByLabel[otherBlock.Label]
		if !found {
			merged = append(merged, otherBlock)

			continue
		}

		existingBlock := merged[i]
		existingBlock.IgnorableErrors = util.MergeSlices(
			existingBlock.IgnorableErrors,
			otherBlock.IgnorableErrors,
		)

		if otherBlock.Message != "" {
			existingBlock.Message = otherBlock.Message
		}

		if otherBlock.Signals != nil {
			if existingBlock.Signals == nil {
				existingBlock.Signals = make(map[string]cty.Value, len(otherBlock.Signals))
			}

			maps.Copy(existingBlock.Signals, otherBlock.Signals)
		}
	}

	return merged
}

// lastBlockPerLabel returns one block for each label in blocks: the last block declared with the label, at the
// position of the first. It returns nil when blocks is empty.
func lastBlockPerLabel[B interface{ label() string }](blocks []B) []B {
	if len(blocks) == 0 {
		return nil
	}

	result := make([]B, 0, len(blocks))
	indexByLabel := make(map[string]int, len(blocks))

	for _, block := range blocks {
		if i, found := indexByLabel[block.label()]; found {
			result[i] = block

			continue
		}

		indexByLabel[block.label()] = len(result)
		result = append(result, block)
	}

	return result
}
